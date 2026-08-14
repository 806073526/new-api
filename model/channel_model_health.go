package model

import (
	"fmt"
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
	FailureThreshold            int
	FailureWindowSeconds        int64
	FirstResponseTimeoutSeconds int64
	CooldownSeconds             int64
	MaxCooldownSeconds          int64
	HalfOpenLeaseSeconds        int64
	ActiveProbeIntervalSeconds  int64
}

func DefaultChannelModelHealthConfig() ChannelModelHealthConfig {
	return ChannelModelHealthConfig{
		FailureThreshold:            3,
		FailureWindowSeconds:        60,
		FirstResponseTimeoutSeconds: 0,
		CooldownSeconds:             60,
		MaxCooldownSeconds:          1800,
		HalfOpenLeaseSeconds:        30,
		ActiveProbeIntervalSeconds:  0,
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
	ChannelName          string `json:"channel_name" gorm:"column:channel_name"`
	ChannelStatus        int    `json:"channel_status" gorm:"column:channel_status"`
	HealthRecordExists   bool   `json:"health_record_exists" gorm:"-"`
	LastRequestUsername  string `json:"last_request_username" gorm:"-"`
	LastRequestTokenName string `json:"last_request_token_name" gorm:"-"`
	LastRequestAt        int64  `json:"last_request_at" gorm:"-"`
	ManualDisabled       bool   `json:"manual_disabled" gorm:"-"`
	ManualDisableReason  string `json:"manual_disable_reason" gorm:"-"`
	ManualDisabledBy     string `json:"manual_disabled_by" gorm:"-"`
	ManualDisabledAt     int64  `json:"manual_disabled_at" gorm:"-"`
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
	ChannelId      int                              `json:"channel_id"`
	Total          int64                            `json:"total"`
	Healthy        int64                            `json:"healthy"`
	ManualDisabled int64                            `json:"manual_disabled"`
	Suspect        int64                            `json:"suspect"`
	Open           int64                            `json:"open"`
	Ready          int64                            `json:"ready"`
	HalfOpen       int64                            `json:"half_open"`
	Closed         int64                            `json:"closed"`
	Issues         []ChannelModelHealthSummaryIssue `json:"issues,omitempty"`
	Models         []ChannelModelHealthSummaryModel `json:"models,omitempty"`
}

type ChannelModelHealthSummaryModel struct {
	Group              string                  `json:"group"`
	Model              string                  `json:"model"`
	State              ChannelModelHealthState `json:"state"`
	Ready              bool                    `json:"ready"`
	HealthRecordExists bool                    `json:"health_record_exists"`
	ManualDisabled     bool                    `json:"manual_disabled"`
}

type ChannelModelHealthSummaryIssue struct {
	Group                string                  `json:"group"`
	Model                string                  `json:"model"`
	State                ChannelModelHealthState `json:"state"`
	Ready                bool                    `json:"ready"`
	FailureCount         int                     `json:"failure_count"`
	LastStatusCode       int                     `json:"last_status_code"`
	LastErrorCode        string                  `json:"last_error_code"`
	LastError            string                  `json:"last_error"`
	LastRequestUsername  string                  `json:"last_request_username"`
	LastRequestTokenName string                  `json:"last_request_token_name"`
	LastRequestAt        int64                   `json:"last_request_at"`
	ManualDisabled       bool                    `json:"manual_disabled"`
	ManualDisableReason  string                  `json:"manual_disable_reason"`
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
	cooldown := maxCooldown
	if base <= maxCooldown {
		openCount := int64(health.OpenCount)
		if openCount <= 0 {
			openCount = 1
		}
		if openCount <= maxCooldown/base {
			cooldown = base * openCount
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

type channelModelHealthLastRequest struct {
	ChannelId int    `gorm:"column:channel_id"`
	Group     string `gorm:"column:request_group"`
	Model     string `gorm:"column:model_name"`
	Username  string `gorm:"column:username"`
	TokenName string `gorm:"column:token_name"`
	CreatedAt int64  `gorm:"column:created_at"`
	Id        int    `gorm:"column:id"`
	RequestId string `gorm:"column:request_id"`
}

const channelModelHealthRequestInfoBatchSize = 100

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

func latestChannelModelHealthRequestInfo(keys []channelModelHealthKey) (map[channelModelHealthKey]channelModelHealthLastRequest, error) {
	requests := make(map[channelModelHealthKey]channelModelHealthLastRequest)
	if LOG_DB == nil || len(keys) == 0 {
		return requests, nil
	}

	uniqueKeys := make([]channelModelHealthKey, 0, len(keys))
	seen := make(map[channelModelHealthKey]struct{}, len(keys))
	for _, key := range keys {
		key = normalizeChannelModelHealthKey(key.ChannelId, key.Group, key.Model)
		if key.ChannelId <= 0 || key.Model == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		uniqueKeys = append(uniqueKeys, key)
	}

	for start := 0; start < len(uniqueKeys); start += channelModelHealthRequestInfoBatchSize {
		end := start + channelModelHealthRequestInfoBatchSize
		if end > len(uniqueKeys) {
			end = len(uniqueKeys)
		}
		batch := uniqueKeys[start:end]
		conditions := make([]string, 0, len(batch))
		args := make([]interface{}, 0, 4+len(batch)*3)
		args = append(args, LogTypeConsume, LogTypeError)
		for _, key := range batch {
			conditions = append(conditions, "(channel_id = ? AND model_name = ? AND "+logGroupCol+" = ?)")
			args = append(args, key.ChannelId, key.Model, key.Group)
		}

		args = append(args, LogTypeConsume, LogTypeError)
		query := fmt.Sprintf(`
SELECT l.channel_id, l.model_name, l.%s AS request_group, l.username, l.token_name, l.created_at, l.id, l.request_id
FROM logs AS l
INNER JOIN (
	SELECT channel_id AS latest_channel_id, model_name AS latest_model_name, %s AS latest_group,
		MAX(created_at) AS latest_created_at
	FROM logs
	WHERE type IN (?, ?) AND (%s)
	GROUP BY channel_id, model_name, %s
) AS latest_requests
	ON l.channel_id = latest_requests.latest_channel_id
	AND l.model_name = latest_requests.latest_model_name
	AND l.%s = latest_requests.latest_group
	AND l.created_at = latest_requests.latest_created_at
WHERE l.type IN (?, ?)
ORDER BY l.created_at DESC, l.id DESC, l.request_id DESC`,
			logGroupCol,
			logGroupCol,
			strings.Join(conditions, " OR "),
			logGroupCol,
			logGroupCol,
		)

		var rows []channelModelHealthLastRequest
		if err := LOG_DB.Raw(query, args...).Scan(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			key := normalizeChannelModelHealthKey(row.ChannelId, row.Group, row.Model)
			if _, exists := requests[key]; exists {
				continue
			}
			requests[key] = row
		}
	}

	return requests, nil
}

func attachLatestChannelModelHealthRequestInfo(items []ChannelModelHealthView) error {
	keys := make([]channelModelHealthKey, 0, len(items))
	for _, item := range items {
		keys = append(keys, normalizeChannelModelHealthKey(item.ChannelId, item.Group, item.Model))
	}
	requests, err := latestChannelModelHealthRequestInfo(keys)
	if err != nil {
		return err
	}
	for index := range items {
		key := normalizeChannelModelHealthKey(items[index].ChannelId, items[index].Group, items[index].Model)
		request, exists := requests[key]
		if !exists {
			continue
		}
		items[index].LastRequestUsername = request.Username
		items[index].LastRequestTokenName = request.TokenName
		items[index].LastRequestAt = request.CreatedAt
	}
	return nil
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
		FailureThreshold:            setting.FailureThreshold,
		FailureWindowSeconds:        setting.FailureWindowSeconds,
		FirstResponseTimeoutSeconds: setting.FirstResponseTimeoutSeconds,
		CooldownSeconds:             setting.CooldownSeconds,
		MaxCooldownSeconds:          setting.MaxCooldownSeconds,
		HalfOpenLeaseSeconds:        setting.HalfOpenLeaseSeconds,
		ActiveProbeIntervalSeconds:  setting.ActiveProbeIntervalSeconds,
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

func GetChannelModelFirstResponseTimeoutSeconds(channelId int, model string) int64 {
	if !IsChannelModelHealthEnabled() || IsChannelModelHealthExcluded(channelId, model) {
		return 0
	}
	return GetChannelModelHealthConfig().FirstResponseTimeoutSeconds
}

func IsChannelModelHealthExcluded(channelId int, model string) bool {
	setting := operation_setting.GetChannelModelHealthSetting()
	return setting.IsChannelExcluded(channelId) || setting.IsModelExcluded(model)
}

// GetChannelModelHealthGroups returns every configured group for one channel
// model pair. Channel tests and manual controls use the same group set so a
// selected model always affects all of that channel's groups.
func GetChannelModelHealthGroups(channelId int, model string) ([]string, error) {
	if DB == nil || channelId <= 0 || strings.TrimSpace(model) == "" {
		return []string{}, nil
	}

	var groups []string
	err := DB.Model(&Ability{}).
		Where("channel_id = ? AND model = ?", channelId, strings.TrimSpace(model)).
		Distinct(commonGroupCol).
		Pluck(commonGroupCol, &groups).Error
	if err != nil {
		return nil, err
	}

	uniqueGroups := make(map[string]struct{}, len(groups))
	for _, group := range groups {
		group = strings.TrimSpace(group)
		if group != "" {
			uniqueGroups[group] = struct{}{}
		}
	}
	groups = groups[:0]
	for group := range uniqueGroups {
		groups = append(groups, group)
	}
	sort.Strings(groups)
	return groups, nil
}

func OpenChannelModelHealth(channelId int, model string, now int64) (int, error) {
	model = strings.TrimSpace(model)
	if !IsChannelModelHealthEnabled() {
		return 0, fmt.Errorf("channel model health is disabled")
	}
	if IsChannelModelHealthExcluded(channelId, model) {
		return 0, fmt.Errorf("channel model health is excluded")
	}
	groups, err := GetChannelModelHealthGroups(channelId, model)
	if err != nil {
		return 0, err
	}

	config := GetChannelModelHealthConfig()
	for _, group := range groups {
		key := normalizeChannelModelHealthKey(channelId, group, model)
		channelModelHealthCache.Lock()
		health := channelModelHealthCache.items[key]
		if health == nil {
			health = &ChannelModelHealth{
				ChannelId: channelId,
				Group:     key.Group,
				Model:     key.Model,
				State:     ChannelModelHealthClosed,
			}
			channelModelHealthCache.items[key] = health
		}
		health.open(now, config)
		snapshot := *health
		channelModelHealthCache.Unlock()
		PersistChannelModelHealth(&snapshot)
	}
	return len(groups), nil
}

func RecoverChannelModelHealth(channelId int, model string, now int64) (int, error) {
	model = strings.TrimSpace(model)
	if !IsChannelModelHealthEnabled() {
		return 0, fmt.Errorf("channel model health is disabled")
	}
	if IsChannelModelHealthExcluded(channelId, model) {
		return 0, fmt.Errorf("channel model health is excluded")
	}
	groups, err := GetChannelModelHealthGroups(channelId, model)
	if err != nil {
		return 0, err
	}
	for _, group := range groups {
		ObserveChannelModelSuccess(channelId, group, model, now)
	}
	return len(groups), nil
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

// ListChannelModelHealthProbeCandidates returns health records that are ready
// for a recovery probe. Suspect records are intentionally excluded: they are
// still routable and should continue to recover through normal traffic.
func ListChannelModelHealthProbeCandidates(now int64, limit int) ([]ChannelModelHealth, error) {
	if DB == nil {
		return []ChannelModelHealth{}, nil
	}
	if limit <= 0 {
		limit = 100
	}

	var items []ChannelModelHealth
	err := DB.Where(
		"(state = ? AND cooldown_until <= ?) OR (state = ? AND half_open_lease_until <= ?)",
		ChannelModelHealthOpen,
		now,
		ChannelModelHealthHalfOpen,
		now,
	).Order("id asc").Limit(limit).Find(&items).Error
	return items, err
}

type channelModelHealthAbilityView struct {
	ChannelId     int    `gorm:"column:channel_id"`
	Group         string `gorm:"column:ability_group"`
	Model         string `gorm:"column:model"`
	ChannelName   string `gorm:"column:channel_name"`
	ChannelStatus int    `gorm:"column:channel_status"`
}

// listChannelModelHealthViews merges persisted health observations for current
// channel/model abilities with enabled abilities that have no health record.
// Health records for models removed from a channel are retained for history but
// excluded from display. Synthesized healthy pairs are display-only and are
// never written to channel_model_health.
func listChannelModelHealthViews() ([]ChannelModelHealthView, error) {
	var items []ChannelModelHealthView
	healthQuery := DB.Table("channel_model_health").
		Select("channel_model_health.*, channels.name AS channel_name, channels.status AS channel_status").
		Joins("JOIN abilities ON abilities.channel_id = channel_model_health.channel_id AND abilities." + commonGroupCol + " = channel_model_health." + commonGroupCol + " AND abilities.model = channel_model_health.model").
		Joins("JOIN channels ON channels.id = channel_model_health.channel_id")
	if err := healthQuery.Scan(&items).Error; err != nil {
		return nil, err
	}

	existing := make(map[channelModelHealthKey]struct{}, len(items))
	for index := range items {
		items[index].HealthRecordExists = true
		existing[normalizeChannelModelHealthKey(
			items[index].ChannelId,
			items[index].Group,
			items[index].Model,
		)] = struct{}{}
	}

	var abilities []channelModelHealthAbilityView
	abilityQuery := DB.Table("abilities").
		Select("abilities.channel_id, abilities."+commonGroupCol+" AS ability_group, abilities.model, channels.name AS channel_name, channels.status AS channel_status").
		Joins("JOIN channels ON channels.id = abilities.channel_id").
		Where("abilities.enabled = ?", true)
	if err := abilityQuery.Scan(&abilities).Error; err != nil {
		return nil, err
	}
	for _, ability := range abilities {
		key := normalizeChannelModelHealthKey(ability.ChannelId, ability.Group, ability.Model)
		if _, ok := existing[key]; ok {
			continue
		}
		existing[key] = struct{}{}
		items = append(items, ChannelModelHealthView{
			ChannelModelHealth: ChannelModelHealth{
				ChannelId: key.ChannelId,
				Group:     key.Group,
				Model:     key.Model,
				State:     ChannelModelHealthClosed,
			},
			ChannelName:   ability.ChannelName,
			ChannelStatus: ability.ChannelStatus,
		})
	}
	manualDisables, err := activeChannelModelManualDisables()
	if err != nil {
		return nil, err
	}
	for index := range items {
		manualDisable, exists := manualDisables[normalizeChannelModelManualDisableKey(items[index].ChannelId, items[index].Model)]
		if !exists {
			continue
		}
		items[index].ManualDisabled = true
		items[index].ManualDisableReason = manualDisable.Reason
		items[index].ManualDisabledBy = manualDisable.OperatorName
		items[index].ManualDisabledAt = manualDisable.CreatedAt
	}
	return items, nil
}

func filterChannelModelHealthViewsByChannelIDs(views []ChannelModelHealthView, channelIDs []int) []ChannelModelHealthView {
	if len(channelIDs) == 0 {
		return views
	}
	allowed := make(map[int]struct{}, len(channelIDs))
	for _, channelID := range channelIDs {
		if channelID > 0 {
			allowed[channelID] = struct{}{}
		}
	}
	if len(allowed) == 0 {
		return []ChannelModelHealthView{}
	}
	filtered := views[:0]
	for _, view := range views {
		if _, ok := allowed[view.ChannelId]; ok {
			filtered = append(filtered, view)
		}
	}
	return filtered
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
	items, err := listChannelModelHealthViews()
	if err != nil {
		return nil, 0, err
	}

	modelFilter := strings.ToLower(params.Model)
	filtered := items[:0]
	for _, item := range items {
		if params.ChannelId > 0 && item.ChannelId != params.ChannelId {
			continue
		}
		if params.Group != "" && item.Group != params.Group {
			continue
		}
		if modelFilter != "" && !strings.Contains(strings.ToLower(item.Model), modelFilter) {
			continue
		}
		state := item.State
		if state == "" {
			state = ChannelModelHealthClosed
		}
		if params.State != "" && state != params.State {
			continue
		}
		filtered = append(filtered, item)
	}
	items = filtered
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].UpdatedAt != items[j].UpdatedAt {
			return items[i].UpdatedAt > items[j].UpdatedAt
		}
		if items[i].Id != items[j].Id {
			return items[i].Id > items[j].Id
		}
		if items[i].ChannelId != items[j].ChannelId {
			return items[i].ChannelId < items[j].ChannelId
		}
		if items[i].Group != items[j].Group {
			return items[i].Group < items[j].Group
		}
		return items[i].Model < items[j].Model
	})

	total := int64(len(items))
	start := (params.Page - 1) * params.PageSize
	if start >= len(items) {
		return []ChannelModelHealthView{}, total, nil
	}
	end := start + params.PageSize
	if end > len(items) {
		end = len(items)
	}
	pageItems := items[start:end]
	if err := attachLatestChannelModelHealthRequestInfo(pageItems); err != nil {
		return nil, 0, err
	}
	return pageItems, total, nil
}

func GetChannelModelHealthSummary() ([]ChannelModelHealthSummaryItem, error) {
	return getChannelModelHealthSummary(nil, false)
}

// GetChannelModelHealthSummaryForChannels returns summaries for only the
// requested channels. The channel list uses this to avoid returning model
// details for channels outside the current page.
func GetChannelModelHealthSummaryForChannels(channelIDs []int, includeModels bool) ([]ChannelModelHealthSummaryItem, error) {
	return getChannelModelHealthSummary(channelIDs, includeModels)
}

func getChannelModelHealthSummary(channelIDs []int, includeModels bool) ([]ChannelModelHealthSummaryItem, error) {
	if DB == nil {
		return []ChannelModelHealthSummaryItem{}, nil
	}
	views, err := listChannelModelHealthViews()
	if err != nil {
		return nil, err
	}
	views = filterChannelModelHealthViewsByChannelIDs(views, channelIDs)
	byChannel := make(map[int]*ChannelModelHealthSummaryItem, len(views))
	now := common.GetTimestamp()
	for _, view := range views {
		item := byChannel[view.ChannelId]
		if item == nil {
			item = &ChannelModelHealthSummaryItem{ChannelId: view.ChannelId}
			byChannel[view.ChannelId] = item
		}
		item.Total++
		state := view.State
		if state == "" {
			state = ChannelModelHealthClosed
		}
		if view.ManualDisabled {
			item.ManualDisabled++
		} else if !view.HealthRecordExists && state == ChannelModelHealthClosed {
			item.Healthy++
		}
		switch state {
		case ChannelModelHealthSuspect:
			item.Suspect++
		case ChannelModelHealthOpen:
			if view.CooldownUntil <= now {
				item.Ready++
			} else {
				item.Open++
			}
		case ChannelModelHealthHalfOpen:
			item.HalfOpen++
		case ChannelModelHealthClosed:
			if view.HealthRecordExists {
				item.Closed++
			}
		}

		if view.ManualDisabled || state == ChannelModelHealthSuspect || state == ChannelModelHealthOpen || state == ChannelModelHealthHalfOpen {
			item.Issues = append(item.Issues, ChannelModelHealthSummaryIssue{
				Group:               view.Group,
				Model:               view.Model,
				State:               state,
				Ready:               !view.ManualDisabled && state == ChannelModelHealthOpen && view.CooldownUntil <= now,
				FailureCount:        view.FailureCount,
				LastStatusCode:      view.LastStatusCode,
				LastErrorCode:       view.LastErrorCode,
				LastError:           view.LastError,
				ManualDisabled:      view.ManualDisabled,
				ManualDisableReason: view.ManualDisableReason,
			})
		}
		if includeModels {
			item.Models = append(item.Models, ChannelModelHealthSummaryModel{
				Group:              view.Group,
				Model:              view.Model,
				State:              state,
				Ready:              state == ChannelModelHealthOpen && view.CooldownUntil <= now,
				HealthRecordExists: view.HealthRecordExists,
				ManualDisabled:     view.ManualDisabled,
			})
		}
	}
	issueKeys := make([]channelModelHealthKey, 0)
	for _, item := range byChannel {
		for _, issue := range item.Issues {
			issueKeys = append(issueKeys, normalizeChannelModelHealthKey(item.ChannelId, issue.Group, issue.Model))
		}
	}
	requests, err := latestChannelModelHealthRequestInfo(issueKeys)
	if err != nil {
		return nil, err
	}
	for _, item := range byChannel {
		for index := range item.Issues {
			issue := &item.Issues[index]
			key := normalizeChannelModelHealthKey(item.ChannelId, issue.Group, issue.Model)
			request, exists := requests[key]
			if !exists {
				continue
			}
			issue.LastRequestUsername = request.Username
			issue.LastRequestTokenName = request.TokenName
			issue.LastRequestAt = request.CreatedAt
		}
	}
	items := make([]ChannelModelHealthSummaryItem, 0, len(byChannel))
	for _, item := range byChannel {
		if includeModels {
			sort.SliceStable(item.Models, func(i, j int) bool {
				if item.Models[i].Group != item.Models[j].Group {
					return item.Models[i].Group < item.Models[j].Group
				}
				return item.Models[i].Model < item.Models[j].Model
			})
		}
		sort.SliceStable(item.Issues, func(i, j int) bool {
			if item.Issues[i].Ready != item.Issues[j].Ready {
				return !item.Issues[i].Ready
			}
			if item.Issues[i].State != item.Issues[j].State {
				return item.Issues[i].State < item.Issues[j].State
			}
			if item.Issues[i].Model != item.Issues[j].Model {
				return item.Issues[i].Model < item.Issues[j].Model
			}
			return item.Issues[i].Group < item.Issues[j].Group
		})
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
	case types.ErrorCodeFirstResponseTimeout:
		return true
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
