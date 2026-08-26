package model

import (
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

type ChannelAvailabilityState string

const (
	ChannelAvailabilityAvailable   ChannelAvailabilityState = "available"
	ChannelAvailabilityUnavailable ChannelAvailabilityState = "unavailable"
	ChannelAvailabilityUnknown     ChannelAvailabilityState = "unknown"
)

const (
	ChannelAvailabilitySourceActiveTest = "active_test"
	ChannelAvailabilitySourceRequest    = "request"
	ChannelAvailabilitySourceUnknown    = "unknown"

	ChannelAvailabilityReasonFresh              = "fresh"
	ChannelAvailabilityReasonNoObservation      = "no_observation"
	ChannelAvailabilityReasonStale              = "stale"
	ChannelAvailabilityReasonRecentFailure      = "recent_failure"
	ChannelAvailabilityReasonChannelDisabled    = "channel_disabled"
	ChannelAvailabilityReasonTemporarilyBlocked = "temporarily_blocked"
	ChannelAvailabilityReasonNoCandidate        = "no_candidate"
)

type ChannelAvailabilityModel struct {
	Model      string                   `json:"model"`
	State      ChannelAvailabilityState `json:"state"`
	Reason     string                   `json:"reason"`
	LatencyMs  int64                    `json:"latency_ms,omitempty"`
	Source     string                   `json:"source"`
	ObservedAt int64                    `json:"observed_at,omitempty"`
}

type ChannelAvailabilityGroup struct {
	Group  string                     `json:"group"`
	Models []ChannelAvailabilityModel `json:"models"`
}

type ChannelAvailabilitySnapshot struct {
	Groups           []ChannelAvailabilityGroup `json:"groups"`
	FreshnessSeconds int64                      `json:"freshness_seconds"`
	GeneratedAt      int64                      `json:"generated_at"`
}

type ChannelModelTestTarget struct {
	ChannelId int    `gorm:"column:channel_id"`
	Group     string `gorm:"column:target_group"`
	Model     string `gorm:"column:model"`
}

type channelAvailabilityCandidate struct {
	ChannelStatus int
	ManualDisable bool
	Health        *ChannelModelHealth
}

type channelAvailabilityEvaluation struct {
	State      ChannelAvailabilityState
	Reason     string
	LatencyMs  int64
	Source     string
	ObservedAt int64
}

func channelModelHealthObservedAt(health *ChannelModelHealth) int64 {
	if health == nil {
		return 0
	}
	if health.LastSuccessAt > health.LastFailureAt {
		return health.LastSuccessAt
	}
	return health.LastFailureAt
}

func channelModelHealthSource(health *ChannelModelHealth) string {
	if health == nil || strings.TrimSpace(health.LastObservationSource) == "" {
		return "unknown"
	}
	return health.LastObservationSource
}

func isFreshChannelModelSuccess(health *ChannelModelHealth, now, freshnessSeconds int64) bool {
	return health != nil &&
		health.LastSuccessAt > health.LastFailureAt &&
		health.LastSuccessAt > 0 &&
		health.LastSuccessAt <= now &&
		now-health.LastSuccessAt <= freshnessSeconds
}

func evaluateChannelAvailabilityDetails(candidates []channelAvailabilityCandidate, now, freshnessSeconds int64) channelAvailabilityEvaluation {
	if len(candidates) == 0 {
		return channelAvailabilityEvaluation{
			State:  ChannelAvailabilityUnavailable,
			Reason: ChannelAvailabilityReasonNoCandidate,
			Source: ChannelAvailabilitySourceUnknown,
		}
	}

	available := channelAvailabilityEvaluation{
		State:  ChannelAvailabilityUnknown,
		Reason: ChannelAvailabilityReasonNoObservation,
		Source: ChannelAvailabilitySourceUnknown,
	}
	latest := channelAvailabilityEvaluation{
		State:  ChannelAvailabilityUnknown,
		Reason: ChannelAvailabilityReasonNoObservation,
		Source: ChannelAvailabilitySourceUnknown,
	}
	activeCandidates := 0
	blockedCandidates := 0
	disabledCandidates := 0
	for _, candidate := range candidates {
		if candidate.ChannelStatus != common.ChannelStatusEnabled || candidate.ManualDisable {
			disabledCandidates++
			continue
		}
		activeCandidates++
		health := candidate.Health
		observedAt := channelModelHealthObservedAt(health)
		if observedAt > latest.ObservedAt {
			reason := ChannelAvailabilityReasonStale
			if health != nil && health.LastFailureAt >= health.LastSuccessAt && health.LastFailureAt > 0 {
				reason = ChannelAvailabilityReasonRecentFailure
			}
			latest = channelAvailabilityEvaluation{
				State:      ChannelAvailabilityUnknown,
				Reason:     reason,
				LatencyMs:  health.LastLatencyMs,
				Source:     channelModelHealthSource(health),
				ObservedAt: observedAt,
			}
		}
		if health != nil && health.State == ChannelModelHealthOpen && health.CooldownUntil > now {
			blockedCandidates++
			continue
		}
		if isFreshChannelModelSuccess(health, now, freshnessSeconds) && health.LastSuccessAt > available.ObservedAt {
			available = channelAvailabilityEvaluation{
				State:      ChannelAvailabilityAvailable,
				Reason:     ChannelAvailabilityReasonFresh,
				LatencyMs:  health.LastLatencyMs,
				Source:     channelModelHealthSource(health),
				ObservedAt: health.LastSuccessAt,
			}
		}
	}

	if available.State == ChannelAvailabilityAvailable {
		return available
	}
	if activeCandidates == 0 {
		if disabledCandidates == len(candidates) {
			latest.State = ChannelAvailabilityUnavailable
			latest.Reason = ChannelAvailabilityReasonChannelDisabled
			return latest
		}
		latest.State = ChannelAvailabilityUnavailable
		latest.Reason = ChannelAvailabilityReasonTemporarilyBlocked
		return latest
	}
	if blockedCandidates == activeCandidates {
		latest.State = ChannelAvailabilityUnavailable
		latest.Reason = ChannelAvailabilityReasonTemporarilyBlocked
		return latest
	}
	if latest.ObservedAt == 0 {
		latest.Reason = ChannelAvailabilityReasonNoObservation
	}
	return latest
}

func evaluateChannelAvailability(candidates []channelAvailabilityCandidate, now, freshnessSeconds int64) ChannelAvailabilityState {
	return evaluateChannelAvailabilityDetails(candidates, now, freshnessSeconds).State
}

func ChannelAvailabilityFreshnessSeconds() int64 {
	monitor := operation_setting.GetMonitorSetting()
	intervalSeconds := int64(monitor.AutoTestChannelMinutes * 60)
	if intervalSeconds <= 0 {
		intervalSeconds = 10 * 60
	}
	timeoutSeconds := GetChannelModelHealthConfig().FirstResponseTimeoutSeconds
	if timeoutSeconds < 0 {
		timeoutSeconds = 0
	}
	freshnessSeconds := intervalSeconds*2 + timeoutSeconds
	if freshnessSeconds < 5*60 {
		freshnessSeconds = 5 * 60
	}
	if freshnessSeconds > 60*60 {
		freshnessSeconds = 60 * 60
	}
	return freshnessSeconds
}

type channelAvailabilityAbility struct {
	ChannelId     int    `gorm:"column:channel_id"`
	Group         string `gorm:"column:availability_group"`
	Model         string `gorm:"column:model"`
	ChannelStatus int    `gorm:"column:channel_status"`
}

func GetChannelModelTestTargets() ([]ChannelModelTestTarget, error) {
	if DB == nil {
		return []ChannelModelTestTarget{}, nil
	}
	var targets []ChannelModelTestTarget
	err := DB.Table("abilities").
		Select("channel_id, "+commonGroupCol+" AS target_group, model").
		Where("enabled = ?", true).
		Order("channel_id asc, " + commonGroupCol + " asc, model asc").
		Scan(&targets).Error
	return targets, err
}

func GetChannelAvailabilitySnapshot(now int64) (ChannelAvailabilitySnapshot, error) {
	return getChannelAvailabilitySnapshot(now, nil)
}

func GetChannelAvailabilitySnapshotForGroups(now int64, allowedGroups []string) (ChannelAvailabilitySnapshot, error) {
	allowed := make(map[string]struct{}, len(allowedGroups))
	for _, group := range allowedGroups {
		group = strings.TrimSpace(group)
		if group != "" {
			allowed[group] = struct{}{}
		}
	}
	return getChannelAvailabilitySnapshot(now, allowed)
}

func getChannelAvailabilitySnapshot(now int64, allowedGroups map[string]struct{}) (ChannelAvailabilitySnapshot, error) {
	if now <= 0 {
		now = common.GetTimestamp()
	}
	snapshot := ChannelAvailabilitySnapshot{
		Groups:           []ChannelAvailabilityGroup{},
		FreshnessSeconds: ChannelAvailabilityFreshnessSeconds(),
		GeneratedAt:      now,
	}
	if DB == nil {
		return snapshot, nil
	}

	var abilities []channelAvailabilityAbility
	if err := DB.Table("abilities").
		Select("abilities.channel_id, abilities."+commonGroupCol+" AS availability_group, abilities.model, channels.status AS channel_status").
		Joins("JOIN channels ON channels.id = abilities.channel_id").
		Where("abilities.enabled = ?", true).
		Scan(&abilities).Error; err != nil {
		return snapshot, err
	}

	manualDisables, err := activeChannelModelManualDisables()
	if err != nil {
		return snapshot, err
	}
	byGroup := make(map[string]map[string][]channelAvailabilityCandidate)
	for _, ability := range abilities {
		group := strings.TrimSpace(ability.Group)
		modelName := strings.TrimSpace(ability.Model)
		if group == "" || modelName == "" {
			continue
		}
		if allowedGroups != nil {
			if _, ok := allowedGroups[group]; !ok {
				continue
			}
		}
		if _, ok := byGroup[group]; !ok {
			byGroup[group] = make(map[string][]channelAvailabilityCandidate)
		}
		key := normalizeChannelModelManualDisableKey(ability.ChannelId, modelName)
		_, manualDisabled := manualDisables[key]
		byGroup[group][modelName] = append(byGroup[group][modelName], channelAvailabilityCandidate{
			ChannelStatus: ability.ChannelStatus,
			ManualDisable: manualDisabled,
			Health:        GetChannelModelHealth(ability.ChannelId, group, modelName),
		})
	}

	groups := make([]string, 0, len(byGroup))
	for group := range byGroup {
		groups = append(groups, group)
	}
	sort.Strings(groups)
	for _, group := range groups {
		modelsByName := byGroup[group]
		modelNames := make([]string, 0, len(modelsByName))
		for modelName := range modelsByName {
			modelNames = append(modelNames, modelName)
		}
		sort.Strings(modelNames)
		groupSnapshot := ChannelAvailabilityGroup{
			Group:  group,
			Models: make([]ChannelAvailabilityModel, 0, len(modelNames)),
		}
		for _, modelName := range modelNames {
			evaluation := evaluateChannelAvailabilityDetails(modelsByName[modelName], now, snapshot.FreshnessSeconds)
			groupSnapshot.Models = append(groupSnapshot.Models, ChannelAvailabilityModel{
				Model:      modelName,
				State:      evaluation.State,
				Reason:     evaluation.Reason,
				LatencyMs:  evaluation.LatencyMs,
				Source:     evaluation.Source,
				ObservedAt: evaluation.ObservedAt,
			})
		}
		snapshot.Groups = append(snapshot.Groups, groupSnapshot)
	}
	return snapshot, nil
}
