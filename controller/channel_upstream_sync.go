package controller

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	upstreamHubBillingBucketSeconds   = 300
	upstreamHubBillingMaxRangeSeconds = 7 * 24 * 60 * 60
)

type upstreamHubIdentity struct {
	ChannelID      int    `json:"channel_id"`
	ChannelName    string `json:"channel_name"`
	BaseURL        string `json:"base_url"`
	KeyFingerprint string `json:"key_fingerprint"`
	Priority       int64  `json:"priority"`
}

type upstreamHubMetricInput struct {
	ChannelID          int      `json:"channel_id" binding:"required"`
	UpstreamChannelID  int      `json:"upstream_channel_id"`
	UpstreamGroup      string   `json:"upstream_group"`
	UpstreamRatio      float64  `json:"upstream_ratio"`
	UpstreamBalance    *float64 `json:"upstream_balance"`
	RatioUpdatedTime   int64    `json:"ratio_updated_time"`
	BalanceUpdatedTime int64    `json:"balance_updated_time"`
	SyncStatus         string   `json:"sync_status"`
	SyncError          string   `json:"sync_error"`
}

type upstreamHubMetricsRequest struct {
	Items []upstreamHubMetricInput `json:"items"`
}

type upstreamHubPriorityInput struct {
	ChannelID     int    `json:"channel_id" binding:"required"`
	Priority      int64  `json:"priority"`
	ExpectedPrior *int64 `json:"expected_priority,omitempty"`
}

type upstreamHubPriorityRequest struct {
	Items []upstreamHubPriorityInput `json:"items"`
}

type upstreamHubBillingAggregateRequest struct {
	StartAt       int64 `json:"start_at" binding:"required"`
	EndAt         int64 `json:"end_at" binding:"required"`
	BucketSeconds int   `json:"bucket_seconds" binding:"required"`
}

type upstreamHubBillingDetailsRequest struct {
	StartAt  int64 `json:"start_at" binding:"required"`
	EndAt    int64 `json:"end_at" binding:"required"`
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
}

func GetUpstreamHubBillingAggregate(c *gin.Context) {
	var request upstreamHubBillingAggregateRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid billing aggregation request"})
		return
	}
	if request.StartAt <= 0 || request.EndAt <= request.StartAt || request.EndAt-request.StartAt > upstreamHubBillingMaxRangeSeconds {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid billing aggregation range"})
		return
	}
	if request.BucketSeconds != upstreamHubBillingBucketSeconds {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "unsupported billing bucket_seconds"})
		return
	}

	items, err := model.GetUpstreamHubBillingBuckets(c.Request.Context(), request.StartAt, request.EndAt, request.BucketSeconds)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if common.QuotaPerUnit <= 0 {
		common.ApiError(c, errors.New("invalid quota_per_unit"))
		return
	}
	channelNames := upstreamHubChannelNames()
	for i := range items {
		items[i].ChannelName = channelNames[items[i].ChannelID]
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"source":         "new-api",
		"start_at":       request.StartAt,
		"end_at":         request.EndAt,
		"bucket_seconds": request.BucketSeconds,
		"quota_per_unit": common.QuotaPerUnit,
		"complete":       true,
		"items":          items,
	}})
}

func GetUpstreamHubBillingDetails(c *gin.Context) {
	var request upstreamHubBillingDetailsRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid billing detail request"})
		return
	}
	if request.StartAt <= 0 || request.EndAt <= request.StartAt || request.EndAt-request.StartAt > upstreamHubBillingMaxRangeSeconds {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid billing detail range"})
		return
	}
	if request.Page < 1 {
		request.Page = 1
	}
	if request.PageSize <= 0 {
		request.PageSize = 500
	}
	if request.PageSize > 500 {
		request.PageSize = 500
	}
	items, total, hasMore, err := model.GetUpstreamHubBillingEvents(c.Request.Context(), request.StartAt, request.EndAt, request.Page, request.PageSize)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	channelNames := upstreamHubChannelNames()
	for i := range items {
		items[i].ChannelName = channelNames[items[i].ChannelID]
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"source": "new-api", "start_at": request.StartAt, "end_at": request.EndAt,
		"page": request.Page, "page_size": request.PageSize, "total": total,
		"has_more": hasMore, "complete": true, "items": items,
	}})
}

func buildUpstreamPriorityPlan(metrics []model.ChannelUpstreamMetric, selected []int, base, step int64) []upstreamHubPriorityInput {
	if base == 0 {
		base = 500
	}
	if step == 0 {
		step = 10
	}
	selectedSet := make(map[int]struct{}, len(selected))
	for _, id := range selected {
		selectedSet[id] = struct{}{}
	}
	byChannel := make(map[int]model.ChannelUpstreamMetric)
	for _, metric := range metrics {
		if metric.ChannelId <= 0 || metric.UpstreamRatio <= 0 || math.IsNaN(metric.UpstreamRatio) || math.IsInf(metric.UpstreamRatio, 0) {
			continue
		}
		if len(selectedSet) > 0 {
			if _, ok := selectedSet[metric.ChannelId]; !ok {
				continue
			}
		}
		if previous, ok := byChannel[metric.ChannelId]; !ok || metric.UpstreamRatio < previous.UpstreamRatio {
			byChannel[metric.ChannelId] = metric
		}
	}
	valid := make([]model.ChannelUpstreamMetric, 0, len(byChannel))
	for _, metric := range byChannel {
		valid = append(valid, metric)
	}
	sort.Slice(valid, func(i, j int) bool {
		if valid[i].UpstreamRatio == valid[j].UpstreamRatio {
			return valid[i].ChannelId < valid[j].ChannelId
		}
		return valid[i].UpstreamRatio < valid[j].UpstreamRatio
	})
	plan := make([]upstreamHubPriorityInput, 0, len(valid))
	priority := base
	lastRatio := math.NaN()
	for _, metric := range valid {
		if !math.IsNaN(lastRatio) && metric.UpstreamRatio != lastRatio {
			priority -= step
		}
		plan = append(plan, upstreamHubPriorityInput{ChannelID: metric.ChannelId, Priority: priority})
		lastRatio = metric.UpstreamRatio
	}
	return plan
}

type upstreamHubPriorityInitializeRequest struct {
	ChannelIDs   []int `json:"channel_ids"`
	BasePriority int64 `json:"base_priority"`
	Step         int64 `json:"step"`
}

func InitializeUpstreamHubPriorities(c *gin.Context) {
	var request upstreamHubPriorityInitializeRequest
	if err := c.ShouldBindJSON(&request); err != nil && err.Error() != "EOF" {
		common.ApiError(c, err)
		return
	}
	var metrics []model.ChannelUpstreamMetric
	if err := model.DB.Order("upstream_ratio ASC, channel_id ASC").Find(&metrics).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	plan := buildUpstreamPriorityPlan(metrics, request.ChannelIDs, request.BasePriority, request.Step)
	updated := 0
	for _, item := range plan {
		channel, err := model.GetChannelById(item.ChannelID, true)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		if channel.GetPriority() == item.Priority {
			continue
		}
		priority := item.Priority
		channel.Priority = &priority
		if err := channel.Update(); err != nil {
			common.ApiError(c, err)
			return
		}
		updated++
	}
	model.InitChannelCache()
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"updated": updated, "planned": len(plan)}})
}

func GetUpstreamHubIdentities(c *gin.Context) {
	var channels []*model.Channel
	// Disabled channels remain valid historical identities. Do not filter by
	// status here: disabling routing must not discard the channel's mapping.
	if err := model.DB.Order("id ASC").Find(&channels).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	identities := make([]upstreamHubIdentity, 0)
	for _, channel := range channels {
		if channel == nil {
			continue
		}
		priority := channel.GetPriority()
		for _, key := range channel.GetKeys() {
			fingerprint := model.UpstreamKeyFingerprint(key)
			if fingerprint == "" {
				continue
			}
			identities = append(identities, upstreamHubIdentity{
				ChannelID: channel.Id, ChannelName: channel.Name, BaseURL: channel.GetBaseURL(),
				KeyFingerprint: fingerprint, Priority: priority,
			})
		}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": identities})
}

func upstreamHubChannelNames() map[int]string {
	names := make(map[int]string)
	load := func(db *gorm.DB, target map[int]string) bool {
		if db == nil {
			return false
		}
		var channels []model.Channel
		if err := db.Select("id", "name").Find(&channels).Error; err != nil {
			return false
		}
		for _, channel := range channels {
			target[channel.Id] = channel.Name
		}
		return true
	}
	load(model.DB, names)
	if model.LOG_DB != nil && model.LOG_DB != model.DB {
		fallback := make(map[int]string)
		if load(model.LOG_DB, fallback) {
			for id, name := range fallback {
				if strings.TrimSpace(names[id]) == "" {
					names[id] = name
				}
			}
		}
	}
	return names
}

func IngestUpstreamHubMetrics(c *gin.Context) {
	var request upstreamHubMetricsRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiError(c, err)
		return
	}
	warningRatios, autoDisable := ratio_setting.GetUpstreamWarningSettings()
	for _, input := range request.Items {
		if input.ChannelID <= 0 || input.UpstreamRatio < 0 || math.IsNaN(input.UpstreamRatio) || math.IsInf(input.UpstreamRatio, 0) {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid upstream metric"})
			return
		}
		status := strings.TrimSpace(input.SyncStatus)
		if status == "" {
			status = "matched"
		}
		if err := model.UpsertChannelUpstreamMetric(&model.ChannelUpstreamMetric{
			ChannelId: input.ChannelID, UpstreamChannelId: input.UpstreamChannelID,
			UpstreamGroup: strings.TrimSpace(input.UpstreamGroup), UpstreamRatio: input.UpstreamRatio,
			UpstreamBalance: input.UpstreamBalance, RatioUpdatedTime: input.RatioUpdatedTime,
			BalanceUpdatedTime: input.BalanceUpdatedTime, SyncStatus: status, SyncError: input.SyncError,
		}); err != nil {
			common.ApiError(c, err)
			return
		}
		channel, err := model.GetChannelById(input.ChannelID, true)
		if err == nil && channel.Status == common.ChannelStatusEnabled && shouldAutoDisableUpstreamRatio(channel, input.UpstreamRatio, warningRatios, autoDisable) {
			model.UpdateChannelStatus(
				input.ChannelID,
				"",
				common.ChannelStatusAutoDisabled,
				fmt.Sprintf("upstream ratio %.6g exceeded configured warning ratio", input.UpstreamRatio),
			)
		}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"updated": len(request.Items)}})
}

func shouldAutoDisableUpstreamRatio(channel *model.Channel, upstreamRatio float64, warningRatios map[string]float64, autoDisable map[string]bool) bool {
	if channel == nil || upstreamRatio <= 0 || math.IsNaN(upstreamRatio) || math.IsInf(upstreamRatio, 0) {
		return false
	}
	for _, group := range channel.GetGroups() {
		group = strings.TrimSpace(group)
		if !autoDisable[group] {
			continue
		}
		warningRatio, ok := warningRatios[group]
		if !ok || warningRatio <= 0 || math.IsNaN(warningRatio) || math.IsInf(warningRatio, 0) {
			continue
		}
		if upstreamRatio > warningRatio {
			return true
		}
	}
	return false
}

func ApplyUpstreamHubPriorities(c *gin.Context) {
	var request upstreamHubPriorityRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiError(c, err)
		return
	}
	updated := 0
	for _, input := range request.Items {
		channel, err := model.GetChannelById(input.ChannelID, true)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		if input.ExpectedPrior != nil && channel.GetPriority() != *input.ExpectedPrior {
			c.JSON(http.StatusConflict, gin.H{"success": false, "message": "channel priority changed", "channel_id": input.ChannelID})
			return
		}
		priority := input.Priority
		channel.Priority = &priority
		if err := channel.Update(); err != nil {
			common.ApiError(c, err)
			return
		}
		updated++
	}
	model.InitChannelCache()
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"updated": updated}})
}

func GetUpstreamHubMetric(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var metric model.ChannelUpstreamMetric
	if err := model.DB.Where("channel_id = ?", id).First(&metric).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": metric})
}
