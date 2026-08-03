package model

import (
	"sort"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"gorm.io/gorm/clause"
)

type ChannelModelHealthState string

const (
	ChannelModelHealthClosed   ChannelModelHealthState = "closed"
	ChannelModelHealthSuspect  ChannelModelHealthState = "suspect"
	ChannelModelHealthOpen     ChannelModelHealthState = "open"
	ChannelModelHealthHalfOpen ChannelModelHealthState = "half_open"
)

type ChannelModelHealthConfig struct {
	FailureThreshold     int
	FailureWindowSeconds int64
	CooldownSeconds      int64
	MaxCooldownSeconds   int64
	HalfOpenLeaseSeconds int64
}

func DefaultChannelModelHealthConfig() ChannelModelHealthConfig {
	return ChannelModelHealthConfig{
		FailureThreshold:     3,
		FailureWindowSeconds: 60,
		CooldownSeconds:      60,
		MaxCooldownSeconds:   1800,
		HalfOpenLeaseSeconds: 30,
	}
}

type ChannelModelHealth struct {
	Id                 int64                   `json:"id" gorm:"primaryKey"`
	ChannelId          int                     `json:"channel_id" gorm:"uniqueIndex:uniq_channel_model_health_key,priority:1"`
	Group              string                  `json:"group" gorm:"column:group;type:varchar(255);uniqueIndex:uniq_channel_model_health_key,priority:2"`
	Model              string                  `json:"model" gorm:"type:varchar(255);uniqueIndex:uniq_channel_model_health_key,priority:3"`
	State              ChannelModelHealthState `json:"state" gorm:"type:varchar(16);index"`
	FailureCount       int                     `json:"failure_count"`
	WindowStartedAt    int64                   `json:"window_started_at" gorm:"bigint"`
	OpenCount          int                     `json:"open_count"`
	CooldownUntil      int64                   `json:"cooldown_until" gorm:"bigint;index"`
	HalfOpenLeaseUntil int64                   `json:"half_open_lease_until" gorm:"bigint"`
	LastFailureAt      int64                   `json:"last_failure_at" gorm:"bigint"`
	LastSuccessAt      int64                   `json:"last_success_at" gorm:"bigint"`
	LastStatusCode     int                     `json:"last_status_code"`
	LastErrorCode      string                  `json:"last_error_code" gorm:"type:varchar(128)"`
	LastError          string                  `json:"last_error" gorm:"type:text"`
	CreatedAt          int64                   `json:"created_at" gorm:"bigint"`
	UpdatedAt          int64                   `json:"updated_at" gorm:"bigint;index"`
}

type ChannelModelHealthView struct {
	ChannelModelHealth
	ChannelName   string `json:"channel_name" gorm:"column:channel_name"`
	ChannelStatus int    `json:"channel_status" gorm:"column:channel_status"`
}

type ChannelModelHealthListParams struct {
	ChannelId int
	Group     string
	Model     string
	State     ChannelModelHealthState
	Page      int
	PageSize  int
}

type ChannelModelHealthSummaryItem struct {
	ChannelId int   `json:"channel_id"`
	Total     int64 `json:"total"`
	Suspect   int64 `json:"suspect"`
	Open      int64 `json:"open"`
	HalfOpen  int64 `json:"half_open"`
	Closed    int64 `json:"closed"`
}

func (ChannelModelHealth) TableName() string {
	return "channel_model_health"
}

func (health *ChannelModelHealth) IsRoutable(now int64) bool {
	if health == nil {
		return true
	}
	switch health.State {
	case "", ChannelModelHealthClosed, ChannelModelHealthSuspect:
		return true
	case ChannelModelHealthOpen:
		return health.CooldownUntil <= now
	case ChannelModelHealthHalfOpen:
		return health.HalfOpenLeaseUntil <= now
	default:
		return true
	}
}

// TryAcquire returns whether the candidate may be used and whether this use is
// the single half-open probe after a cooldown.
func (health *ChannelModelHealth) TryAcquire(now int64, config ChannelModelHealthConfig) (bool, bool) {
	if health == nil {
		return true, false
	}
	switch health.State {
	case "", ChannelModelHealthClosed, ChannelModelHealthSuspect:
		return true, false
	case ChannelModelHealthOpen:
		if health.CooldownUntil > now {
			return false, false
		}
		health.State = ChannelModelHealthHalfOpen
		health.HalfOpenLeaseUntil = now + positiveDuration(config.HalfOpenLeaseSeconds, 30)
		return true, true
	case ChannelModelHealthHalfOpen:
		if health.HalfOpenLeaseUntil > now {
			return false, false
		}
		health.HalfOpenLeaseUntil = now + positiveDuration(config.HalfOpenLeaseSeconds, 30)
		return true, true
	default:
		return true, false
	}
}

func (health *ChannelModelHealth) ObserveFailure(now int64, statusCode int, errorCode, message string, config ChannelModelHealthConfig) {
	if health == nil {
		return
	}
	if health.State == ChannelModelHealthHalfOpen || health.State == ChannelModelHealthOpen {
		health.open(now, config)
		health.setLastError(now, statusCode, errorCode, message)
		return
	}

	window := positiveDuration(config.FailureWindowSeconds, 60)
	if health.WindowStartedAt == 0 || now-health.WindowStartedAt >= window {
		health.WindowStartedAt = now
		health.FailureCount = 0
	}
	health.FailureCount++
	health.LastFailureAt = now
	health.setLastError(now, statusCode, errorCode, message)
	threshold := config.FailureThreshold
	if threshold <= 0 {
		threshold = 3
	}
	if health.FailureCount >= threshold {
		health.open(now, config)
	} else {
		health.State = ChannelModelHealthSuspect
	}
}

func (health *ChannelModelHealth) ObserveSuccess(now int64) {
	if health == nil {
		return
	}
	health.State = ChannelModelHealthClosed
	health.FailureCount = 0
	health.WindowStartedAt = 0
	health.CooldownUntil = 0
	health.HalfOpenLeaseUntil = 0
	health.LastSuccessAt = now
	health.UpdatedAt = now
}

func (health *ChannelModelHealth) open(now int64, config ChannelModelHealthConfig) {
	health.State = ChannelModelHealthOpen
	health.OpenCount++
	health.HalfOpenLeaseUntil = 0
	base := positiveDuration(config.CooldownSeconds, 60)
	maxCooldown := positiveDuration(config.MaxCooldownSeconds, 1800)
	cooldown := base
	if cooldown > maxCooldown {
		cooldown = maxCooldown
	} else {
		for reopen := 1; reopen < health.OpenCount && cooldown < maxCooldown; reopen++ {
			if cooldown > maxCooldown/2 {
				cooldown = maxCooldown
				break
			}
			cooldown *= 2
		}
	}
	health.CooldownUntil = now + cooldown
	health.UpdatedAt = now
}

func (health *ChannelModelHealth) setLastError(now int64, statusCode int, errorCode, message string) {
	health.LastStatusCode = statusCode
	health.LastErrorCode = errorCode
	health.LastError = message
	health.UpdatedAt = now
}

func positiveDuration(value, fallback int64) int64 {
	if value > 0 {
		return value
	}
	return fallback
}

type channelModelHealthKey struct {
	ChannelId int
	Group     string
	Model     string
}

var channelModelHealthCache = struct {
	sync.RWMutex
	items map[channelModelHealthKey]*ChannelModelHealth
}{items: make(map[channelModelHealthKey]*ChannelModelHealth)}

func normalizeChannelModelHealthKey(channelId int, group, model string) channelModelHealthKey {
	return channelModelHealthKey{
		ChannelId: channelId,
		Group:     strings.TrimSpace(group),
		Model:     strings.TrimSpace(model),
	}
}

func InitChannelModelHealthCache() {
	if DB == nil {
		return
	}
	var items []*ChannelModelHealth
	if err := DB.Find(&items).Error; err != nil {
		return
	}
	cache := make(map[channelModelHealthKey]*ChannelModelHealth, len(items))
	for _, item := range items {
		cache[normalizeChannelModelHealthKey(item.ChannelId, item.Group, item.Model)] = item
	}
	channelModelHealthCache.Lock()
	channelModelHealthCache.items = cache
	channelModelHealthCache.Unlock()
}

func GetChannelModelHealth(channelId int, group, model string) *ChannelModelHealth {
	key := normalizeChannelModelHealthKey(channelId, group, model)
	channelModelHealthCache.RLock()
	item := channelModelHealthCache.items[key]
	channelModelHealthCache.RUnlock()
	if item != nil {
		copy := *item
		return &copy
	}
	return &ChannelModelHealth{
		ChannelId: channelId,
		Group:     key.Group,
		Model:     key.Model,
		State:     ChannelModelHealthClosed,
	}
}

func IsChannelModelHealthEnabled() bool {
	return operation_setting.GetChannelModelHealthSetting().Enabled
}

func GetChannelModelHealthConfig() ChannelModelHealthConfig {
	setting := operation_setting.GetChannelModelHealthSetting()
	config := ChannelModelHealthConfig{
		FailureThreshold:     setting.FailureThreshold,
		FailureWindowSeconds: setting.FailureWindowSeconds,
		CooldownSeconds:      setting.CooldownSeconds,
		MaxCooldownSeconds:   setting.MaxCooldownSeconds,
		HalfOpenLeaseSeconds: setting.HalfOpenLeaseSeconds,
	}
	defaults := DefaultChannelModelHealthConfig()
	if config.FailureThreshold <= 0 {
		config.FailureThreshold = defaults.FailureThreshold
	}
	if config.FailureWindowSeconds <= 0 {
		config.FailureWindowSeconds = defaults.FailureWindowSeconds
	}
	if config.CooldownSeconds <= 0 {
		config.CooldownSeconds = defaults.CooldownSeconds
	}
	if config.MaxCooldownSeconds <= 0 {
		config.MaxCooldownSeconds = defaults.MaxCooldownSeconds
	}
	if config.HalfOpenLeaseSeconds <= 0 {
		config.HalfOpenLeaseSeconds = defaults.HalfOpenLeaseSeconds
	}
	return config
}

func IsChannelModelHealthExcluded(channelId int, model string) bool {
	setting := operation_setting.GetChannelModelHealthSetting()
	return setting.IsChannelExcluded(channelId) || setting.IsModelExcluded(model)
}

func IsChannelModelRoutable(channelId int, group, model string, now int64) bool {
	if !IsChannelModelHealthEnabled() || IsChannelModelHealthExcluded(channelId, model) {
		return true
	}
	health := GetChannelModelHealth(channelId, group, model)
	return health.IsRoutable(now)
}

func filterChannelModelHealthCandidates(channelIds []int, group, model string, now int64) []int {
	if !IsChannelModelHealthEnabled() || len(channelIds) == 0 {
		return channelIds
	}
	filtered := make([]int, 0, len(channelIds))
	for _, channelId := range channelIds {
		if IsChannelModelRoutable(channelId, group, model, now) {
			filtered = append(filtered, channelId)
		}
	}
	return filtered
}

func TryAcquireChannelModel(channelId int, group, model string, now int64, config ChannelModelHealthConfig) (bool, bool) {
	if !IsChannelModelHealthEnabled() || IsChannelModelHealthExcluded(channelId, model) {
		return true, false
	}
	key := normalizeChannelModelHealthKey(channelId, group, model)
	channelModelHealthCache.Lock()
	health := channelModelHealthCache.items[key]
	if health == nil {
		channelModelHealthCache.Unlock()
		return true, false
	}
	allowed, probe := health.TryAcquire(now, config)
	var snapshot *ChannelModelHealth
	if probe {
		copy := *health
		snapshot = &copy
	}
	channelModelHealthCache.Unlock()
	if snapshot != nil {
		PersistChannelModelHealth(snapshot)
	}
	return allowed, probe
}

func ObserveChannelModelFailure(channelId int, group, model string, err *types.NewAPIError, now int64, config ChannelModelHealthConfig) {
	if !IsChannelModelHealthEnabled() || IsChannelModelHealthExcluded(channelId, model) || err == nil {
		return
	}
	key := normalizeChannelModelHealthKey(channelId, group, model)
	channelModelHealthCache.Lock()
	health := channelModelHealthCache.items[key]
	if health == nil {
		health = &ChannelModelHealth{ChannelId: channelId, Group: key.Group, Model: key.Model, State: ChannelModelHealthClosed}
		channelModelHealthCache.items[key] = health
	}
	health.ObserveFailure(now, err.StatusCode, string(err.GetErrorCode()), err.Error(), config)
	snapshot := *health
	channelModelHealthCache.Unlock()
	PersistChannelModelHealth(&snapshot)
}

func ObserveChannelModelSuccess(channelId int, group, model string, now int64) {
	if !IsChannelModelHealthEnabled() || IsChannelModelHealthExcluded(channelId, model) {
		return
	}
	key := normalizeChannelModelHealthKey(channelId, group, model)
	channelModelHealthCache.Lock()
	health := channelModelHealthCache.items[key]
	if health == nil {
		channelModelHealthCache.Unlock()
		return
	}
	if health.State == ChannelModelHealthClosed && health.FailureCount == 0 {
		channelModelHealthCache.Unlock()
		return
	}
	health.ObserveSuccess(now)
	snapshot := *health
	channelModelHealthCache.Unlock()
	PersistChannelModelHealth(&snapshot)
}

func ListChannelModelHealth(params ChannelModelHealthListParams) ([]ChannelModelHealthView, int64, error) {
	if DB == nil {
		return []ChannelModelHealthView{}, 0, nil
	}
	if params.Page < 1 {
		params.Page = 1
	}
	if params.PageSize <= 0 {
		params.PageSize = 50
	}
	if params.PageSize > 200 {
		params.PageSize = 200
	}
	query := DB.Table("channel_model_health").
		Select("channel_model_health.*, channels.name AS channel_name, channels.status AS channel_status").
		Joins("LEFT JOIN channels ON channels.id = channel_model_health.channel_id")
	if params.ChannelId > 0 {
		query = query.Where("channel_model_health.channel_id = ?", params.ChannelId)
	}
	if params.Group != "" {
		query = query.Where("channel_model_health."+commonGroupCol+" = ?", params.Group)
	}
	if params.Model != "" {
		query = query.Where("channel_model_health.model LIKE ?", "%"+params.Model+"%")
	}
	if params.State != "" {
		query = query.Where("channel_model_health.state = ?", params.State)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []ChannelModelHealthView
	err := query.Order("channel_model_health.updated_at DESC").
		Order("channel_model_health.id DESC").
		Offset((params.Page - 1) * params.PageSize).
		Limit(params.PageSize).
		Scan(&items).Error
	return items, total, err
}

func GetChannelModelHealthSummary() ([]ChannelModelHealthSummaryItem, error) {
	if DB == nil {
		return []ChannelModelHealthSummaryItem{}, nil
	}
	type row struct {
		ChannelId int
		State     ChannelModelHealthState
		Count     int64
	}
	var rows []row
	if err := DB.Table("channel_model_health").
		Select("channel_id, state, COUNT(*) AS count").
		Group("channel_id, state").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	byChannel := make(map[int]*ChannelModelHealthSummaryItem, len(rows))
	for _, row := range rows {
		item := byChannel[row.ChannelId]
		if item == nil {
			item = &ChannelModelHealthSummaryItem{ChannelId: row.ChannelId}
			byChannel[row.ChannelId] = item
		}
		item.Total += row.Count
		switch row.State {
		case ChannelModelHealthSuspect:
			item.Suspect += row.Count
		case ChannelModelHealthOpen:
			item.Open += row.Count
		case ChannelModelHealthHalfOpen:
			item.HalfOpen += row.Count
		case ChannelModelHealthClosed:
			item.Closed += row.Count
		}
	}
	items := make([]ChannelModelHealthSummaryItem, 0, len(byChannel))
	for _, item := range byChannel {
		items = append(items, *item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ChannelId < items[j].ChannelId })
	return items, nil
}

func ResetChannelModelHealth(channelId int, group, model string) error {
	if DB != nil {
		query := DB
		if channelId > 0 {
			query = query.Where("channel_id = ?", channelId)
		}
		if group != "" {
			query = query.Where(commonGroupCol+" = ?", group)
		}
		if model != "" {
			query = query.Where("model = ?", model)
		}
		if err := query.Delete(&ChannelModelHealth{}).Error; err != nil {
			return err
		}
	}
	channelModelHealthCache.Lock()
	for key := range channelModelHealthCache.items {
		if channelId > 0 && key.ChannelId != channelId {
			continue
		}
		if group != "" && key.Group != strings.TrimSpace(group) {
			continue
		}
		if model != "" && key.Model != strings.TrimSpace(model) {
			continue
		}
		delete(channelModelHealthCache.items, key)
	}
	channelModelHealthCache.Unlock()
	return nil
}

func PersistChannelModelHealth(health *ChannelModelHealth) {
	if DB == nil || health == nil {
		return
	}
	if health.CreatedAt == 0 {
		health.CreatedAt = common.GetTimestamp()
	}
	health.UpdatedAt = common.GetTimestamp()
	updates := map[string]interface{}{
		"state":                 health.State,
		"failure_count":         health.FailureCount,
		"window_started_at":     health.WindowStartedAt,
		"open_count":            health.OpenCount,
		"cooldown_until":        health.CooldownUntil,
		"half_open_lease_until": health.HalfOpenLeaseUntil,
		"last_failure_at":       health.LastFailureAt,
		"last_success_at":       health.LastSuccessAt,
		"last_status_code":      health.LastStatusCode,
		"last_error_code":       health.LastErrorCode,
		"last_error":            health.LastError,
		"updated_at":            health.UpdatedAt,
	}
	_ = DB.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "channel_id"},
			{Name: "group"},
			{Name: "model"},
		},
		DoUpdates: clause.Assignments(updates),
	}).Create(health).Error
}

// ShouldObserveChannelModelFailure identifies upstream failures that are
// attributable to a specific channel/model pair. Client validation and
// channel-level errors must not trip a model circuit breaker.
func ShouldObserveChannelModelFailure(err *types.NewAPIError) bool {
	if err == nil || types.IsChannelError(err) || types.IsSkipRetryError(err) {
		return false
	}
	if err.StatusCode == 401 || err.StatusCode == 402 || err.StatusCode == 403 {
		return false
	}
	switch err.GetErrorCode() {
	case types.ErrorCodeInvalidRequest,
		types.ErrorCodeBadRequestBody,
		types.ErrorCodeReadRequestBodyFailed,
		types.ErrorCodeConvertRequestFailed,
		types.ErrorCodeAccessDenied,
		types.ErrorCodeInsufficientUserQuota,
		types.ErrorCodePreConsumeTokenQuotaFailed,
		types.ErrorCodeGetChannelFailed,
		types.ErrorCodeGenRelayInfoFailed:
		return false
	case types.ErrorCodeModelNotFound:
		return true
	case types.ErrorCodeBadResponseStatusCode:
		return err.StatusCode == 408 || err.StatusCode == 429 || err.StatusCode >= 500
	case types.ErrorCodeBadResponse,
		types.ErrorCodeBadResponseBody,
		types.ErrorCodeReadResponseBodyFailed,
		types.ErrorCodeEmptyResponse,
		types.ErrorCodeDoRequestFailed:
		return true
	}
	return err.StatusCode == 408 || err.StatusCode == 429 || err.StatusCode >= 500
}
