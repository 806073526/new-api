package model

import (
	"context"
	"errors"
	"math"
	"sort"

	"github.com/QuantumNous/new-api/common"
)

const (
	UpstreamHubBillingNormalizationExact       = "exact"
	UpstreamHubBillingNormalizationEstimated   = "estimated"
	UpstreamHubBillingNormalizationUnavailable = "unavailable"
)

// UpstreamHubBillingBucket is an immutable aggregate of consumption facts for
// one NewAPI channel, group, model, time bucket, and effective group ratio.
// Different ratios deliberately remain separate so callers never divide a
// mixed quota total by a single ratio.
type UpstreamHubBillingBucket struct {
	BucketStart         int64   `json:"bucket_start"`
	BucketEnd           int64   `json:"bucket_end"`
	ChannelID           int     `json:"channel_id"`
	ChannelName         string  `json:"channel_name"`
	Group               string  `json:"group"`
	ModelName           string  `json:"model_name"`
	EffectiveGroupRatio float64 `json:"effective_group_ratio"`
	RatioSource         string  `json:"ratio_source"`
	NormalizationStatus string  `json:"normalization_status"`
	ConsumeQuota        int64   `json:"consume_quota"`
	RefundQuota         int64   `json:"refund_quota"`
	NetQuota            int64   `json:"net_quota"`
	EventCount          int64   `json:"event_count"`
}

// UpstreamHubBillingEvent is the audit-safe projection of one NewAPI log. The
// request body is intentionally omitted; source_log_id plus request IDs let an
// operator cross-check the original log without copying user content out of
// NewAPI.
type UpstreamHubBillingEvent struct {
	SourceLogID         int64   `json:"source_log_id"`
	CreatedAt           int64   `json:"created_at"`
	EventType           string  `json:"event_type"`
	ChannelID           int     `json:"channel_id"`
	ChannelName         string  `json:"channel_name"`
	Group               string  `json:"group"`
	ModelName           string  `json:"model_name"`
	EffectiveGroupRatio float64 `json:"effective_group_ratio"`
	RatioSource         string  `json:"ratio_source"`
	NormalizationStatus string  `json:"normalization_status"`
	Quota               int64   `json:"quota"`
	UserID              int     `json:"user_id"`
	TokenName           string  `json:"token_name,omitempty"`
	RequestID           string  `json:"request_id,omitempty"`
	UpstreamRequestID   string  `json:"upstream_request_id,omitempty"`
}

// UpstreamHubPersonalUsageBucket aggregates consumption made by every Root
// user. Root user IDs are resolved from the User table at query time so no
// administrator ID needs to be configured in either deployment.
type UpstreamHubPersonalUsageBucket struct {
	BucketStart  int64 `json:"bucket_start"`
	BucketEnd    int64 `json:"bucket_end"`
	ConsumeQuota int64 `json:"consume_quota"`
	RefundQuota  int64 `json:"refund_quota"`
	NetQuota     int64 `json:"net_quota"`
	EventCount   int64 `json:"event_count"`
}

type upstreamHubBillingBucketKey struct {
	bucketStart         int64
	channelID           int
	group               string
	modelName           string
	effectiveRatioBits  uint64
	ratioSource         string
	normalizationStatus string
}

// GetUpstreamHubBillingBuckets reads only the consume and refund log fields
// needed for settlement. JSON extraction is intentionally done in Go: the
// Other column has to work on SQLite, MySQL, PostgreSQL, and ClickHouse.
func GetUpstreamHubBillingBuckets(ctx context.Context, startAt, endAt int64, bucketSeconds int) ([]UpstreamHubBillingBucket, error) {
	if LOG_DB == nil {
		return nil, errors.New("log database is not initialized")
	}
	if startAt < 0 || endAt <= startAt || bucketSeconds <= 0 {
		return nil, errors.New("invalid billing aggregation range")
	}

	groupColumn := logGroupCol
	if groupColumn == "" {
		groupColumn = "`group`"
	}
	var logs []*Log
	err := LOG_DB.WithContext(ctx).
		Model(&Log{}).
		Select("created_at, type, quota, channel_id, "+groupColumn+", model_name, other").
		Where("type IN ?", []int{LogTypeConsume, LogTypeRefund}).
		Where("created_at >= ? AND created_at < ?", startAt, endAt).
		Find(&logs).Error
	if err != nil {
		return nil, err
	}
	return AggregateUpstreamHubBillingLogs(logs, startAt, endAt, bucketSeconds)
}

// GetUpstreamHubPersonalUsageBuckets aggregates consume/refund logs for all
// Root users. Missing database handles or Root users are reported as an
// incomplete optional dataset rather than failing the sales aggregate.
func GetUpstreamHubPersonalUsageBuckets(ctx context.Context, startAt, endAt int64, bucketSeconds int) ([]UpstreamHubPersonalUsageBucket, bool, error) {
	if startAt < 0 || endAt <= startAt || bucketSeconds <= 0 {
		return nil, false, errors.New("invalid personal usage aggregation range")
	}
	if DB == nil || LOG_DB == nil {
		return []UpstreamHubPersonalUsageBucket{}, false, nil
	}
	var rootIDs []int
	if err := DB.Model(&User{}).Where("role = ?", common.RoleRootUser).Pluck("id", &rootIDs).Error; err != nil {
		return nil, false, err
	}
	if len(rootIDs) == 0 {
		return []UpstreamHubPersonalUsageBucket{}, false, nil
	}
	var logs []*Log
	if err := LOG_DB.WithContext(ctx).
		Model(&Log{}).
		Select("created_at, type, quota, user_id").
		Where("type IN ?", []int{LogTypeConsume, LogTypeRefund}).
		Where("created_at >= ? AND created_at < ?", startAt, endAt).
		Where("user_id IN ?", rootIDs).
		Find(&logs).Error; err != nil {
		return nil, false, err
	}
	items, err := AggregateUpstreamHubPersonalUsageLogs(logs, startAt, endAt, bucketSeconds)
	if err != nil {
		return nil, false, err
	}
	return items, true, nil
}

// AggregateUpstreamHubPersonalUsageLogs turns Root-user logs into idempotent
// time buckets. The caller is responsible for filtering the logs to Root
// users; keeping this pure makes the arithmetic easy to test and reuse.
func AggregateUpstreamHubPersonalUsageLogs(logs []*Log, startAt, endAt int64, bucketSeconds int) ([]UpstreamHubPersonalUsageBucket, error) {
	if startAt < 0 || endAt <= startAt || bucketSeconds <= 0 {
		return nil, errors.New("invalid personal usage aggregation range")
	}
	byStart := make(map[int64]*UpstreamHubPersonalUsageBucket)
	for _, log := range logs {
		if log == nil || log.CreatedAt < startAt || log.CreatedAt >= endAt {
			continue
		}
		if log.Type != LogTypeConsume && log.Type != LogTypeRefund {
			continue
		}
		bucketStart := log.CreatedAt - log.CreatedAt%int64(bucketSeconds)
		bucket := byStart[bucketStart]
		if bucket == nil {
			bucket = &UpstreamHubPersonalUsageBucket{BucketStart: bucketStart, BucketEnd: bucketStart + int64(bucketSeconds)}
			byStart[bucketStart] = bucket
		}
		quota := int64(log.Quota)
		if quota < 0 {
			quota = -quota
		}
		if log.Type == LogTypeConsume {
			bucket.ConsumeQuota += quota
		} else {
			bucket.RefundQuota += quota
		}
		bucket.EventCount++
	}
	items := make([]UpstreamHubPersonalUsageBucket, 0, len(byStart))
	for _, bucket := range byStart {
		bucket.NetQuota = bucket.ConsumeQuota - bucket.RefundQuota
		items = append(items, *bucket)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].BucketStart < items[j].BucketStart })
	return items, nil
}

func GetUpstreamHubBillingEvents(ctx context.Context, startAt, endAt int64, page, pageSize int) ([]UpstreamHubBillingEvent, int64, bool, error) {
	if LOG_DB == nil {
		return nil, 0, false, errors.New("log database is not initialized")
	}
	if startAt < 0 || endAt <= startAt {
		return nil, 0, false, errors.New("invalid billing detail range")
	}
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 500
	}
	if pageSize > 500 {
		pageSize = 500
	}
	groupColumn := logGroupCol
	if groupColumn == "" {
		groupColumn = "`group`"
	}
	query := LOG_DB.WithContext(ctx).
		Model(&Log{}).
		Where("type IN ?", []int{LogTypeConsume, LogTypeRefund}).
		Where("created_at >= ? AND created_at < ?", startAt, endAt)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, false, err
	}
	var logs []*Log
	if err := query.Select("id, created_at, type, quota, user_id, channel_id, " + groupColumn + ", model_name, token_name, request_id, upstream_request_id, other").
		Order("created_at DESC, id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&logs).Error; err != nil {
		return nil, 0, false, err
	}
	events := make([]UpstreamHubBillingEvent, 0, len(logs))
	for _, log := range logs {
		if log == nil {
			continue
		}
		ratio, source, status := upstreamHubEffectiveGroupRatio(log.Other)
		eventType := "consume"
		if log.Type == LogTypeRefund {
			eventType = "refund"
		}
		quota := int64(log.Quota)
		if quota < 0 {
			quota = -quota
		}
		events = append(events, UpstreamHubBillingEvent{
			SourceLogID: int64(log.Id), CreatedAt: log.CreatedAt, EventType: eventType,
			ChannelID: log.ChannelId, Group: log.Group, ModelName: log.ModelName,
			EffectiveGroupRatio: ratio, RatioSource: source, NormalizationStatus: status,
			Quota: quota, UserID: log.UserId, TokenName: log.TokenName,
			RequestID: log.RequestId, UpstreamRequestID: log.UpstreamRequestId,
		})
	}
	return events, total, int64((page-1)*pageSize+len(events)) < total, nil
}

// AggregateUpstreamHubBillingLogs turns log records into idempotent time-bucket
// rows for the UpstreamHub settlement ledger.
func AggregateUpstreamHubBillingLogs(logs []*Log, startAt, endAt int64, bucketSeconds int) ([]UpstreamHubBillingBucket, error) {
	if startAt < 0 || endAt <= startAt || bucketSeconds <= 0 {
		return nil, errors.New("invalid billing aggregation range")
	}

	buckets := make(map[upstreamHubBillingBucketKey]*UpstreamHubBillingBucket)
	for _, log := range logs {
		if log == nil || log.CreatedAt < startAt || log.CreatedAt >= endAt {
			continue
		}
		if log.Type != LogTypeConsume && log.Type != LogTypeRefund {
			continue
		}

		ratio, source, status := upstreamHubEffectiveGroupRatio(log.Other)
		bucketStart := log.CreatedAt - log.CreatedAt%int64(bucketSeconds)
		key := upstreamHubBillingBucketKey{
			bucketStart:         bucketStart,
			channelID:           log.ChannelId,
			group:               log.Group,
			modelName:           log.ModelName,
			effectiveRatioBits:  math.Float64bits(ratio),
			ratioSource:         source,
			normalizationStatus: status,
		}
		bucket := buckets[key]
		if bucket == nil {
			bucket = &UpstreamHubBillingBucket{
				BucketStart:         bucketStart,
				BucketEnd:           bucketStart + int64(bucketSeconds),
				ChannelID:           log.ChannelId,
				Group:               log.Group,
				ModelName:           log.ModelName,
				EffectiveGroupRatio: ratio,
				RatioSource:         source,
				NormalizationStatus: status,
			}
			buckets[key] = bucket
		}

		quota := int64(log.Quota)
		if quota < 0 {
			quota = -quota
		}
		if log.Type == LogTypeConsume {
			bucket.ConsumeQuota += quota
		} else {
			bucket.RefundQuota += quota
		}
		bucket.EventCount++
	}

	items := make([]UpstreamHubBillingBucket, 0, len(buckets))
	for _, bucket := range buckets {
		bucket.NetQuota = bucket.ConsumeQuota - bucket.RefundQuota
		items = append(items, *bucket)
	}
	sort.Slice(items, func(i, j int) bool {
		left, right := items[i], items[j]
		switch {
		case left.BucketStart != right.BucketStart:
			return left.BucketStart < right.BucketStart
		case left.ChannelID != right.ChannelID:
			return left.ChannelID < right.ChannelID
		case left.Group != right.Group:
			return left.Group < right.Group
		case left.ModelName != right.ModelName:
			return left.ModelName < right.ModelName
		case left.EffectiveGroupRatio != right.EffectiveGroupRatio:
			return left.EffectiveGroupRatio < right.EffectiveGroupRatio
		case left.RatioSource != right.RatioSource:
			return left.RatioSource < right.RatioSource
		default:
			return left.NormalizationStatus < right.NormalizationStatus
		}
	})
	return items, nil
}

func upstreamHubEffectiveGroupRatio(other string) (float64, string, string) {
	values := make(map[string]interface{})
	if other != "" && common.UnmarshalJsonStr(other, &values) == nil {
		if ratio, present := upstreamHubOtherRatio(values["user_group_ratio"], false); present {
			return ratio, "user_group_ratio", upstreamHubRatioStatus(ratio)
		}
		if ratio, present := upstreamHubOtherRatio(values["group_ratio"], true); present {
			return ratio, "group_ratio", upstreamHubRatioStatus(ratio)
		}
	}
	return 1, "default", UpstreamHubBillingNormalizationEstimated
}

func upstreamHubOtherRatio(value interface{}, allowZero bool) (float64, bool) {
	ratio, ok := value.(float64)
	if !ok || ratio < 0 || (!allowZero && ratio == 0) || math.IsNaN(ratio) || math.IsInf(ratio, 0) {
		return 0, false
	}
	return ratio, true
}

func upstreamHubRatioStatus(ratio float64) string {
	if ratio > 0 {
		return UpstreamHubBillingNormalizationExact
	}
	return UpstreamHubBillingNormalizationUnavailable
}
