package model

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testNewAPIError(statusCode int, errorCode types.ErrorCode) *types.NewAPIError {
	return types.NewErrorWithStatusCode(errors.New("upstream error"), errorCode, statusCode)
}

func TestChannelModelHealthOpensAfterTransientFailureThreshold(t *testing.T) {
	health := ChannelModelHealth{
		State: ChannelModelHealthClosed,
	}
	config := DefaultChannelModelHealthConfig()
	config.FailureThreshold = 3

	health.ObserveFailure(100, 503, "upstream_error", "temporarily unavailable", config)
	assert.Equal(t, ChannelModelHealthSuspect, health.State)
	assert.True(t, health.IsRoutable(100))

	health.ObserveFailure(101, 503, "upstream_error", "temporarily unavailable", config)
	assert.Equal(t, ChannelModelHealthSuspect, health.State)

	health.ObserveFailure(102, 503, "upstream_error", "temporarily unavailable", config)
	assert.Equal(t, ChannelModelHealthOpen, health.State)
	assert.False(t, health.IsRoutable(102))
	assert.Greater(t, health.CooldownUntil, int64(102))
}

func TestChannelModelHealthHalfOpenAllowsOneRequest(t *testing.T) {
	health := ChannelModelHealth{
		State:         ChannelModelHealthOpen,
		CooldownUntil: 200,
	}
	config := DefaultChannelModelHealthConfig()

	allowed, probe := health.TryAcquire(199, config)
	assert.False(t, allowed)
	assert.False(t, probe)
	assert.False(t, health.IsRoutable(199))
	assert.True(t, health.IsRoutable(200))

	allowed, probe = health.TryAcquire(200, config)
	require.True(t, allowed)
	assert.True(t, probe)
	assert.Equal(t, ChannelModelHealthHalfOpen, health.State)

	allowed, probe = health.TryAcquire(201, config)
	assert.False(t, allowed)
	assert.False(t, probe)
	assert.False(t, health.IsRoutable(201))
	assert.True(t, health.IsRoutable(230))
}

func TestFilterChannelModelHealthCandidatesOnlyExcludesFailedPair(t *testing.T) {
	setting := operation_setting.GetChannelModelHealthSetting()
	previous := *setting
	setting.Enabled = true
	setting.ExcludedChannelIds = nil
	setting.ExcludedModels = nil
	t.Cleanup(func() { *setting = previous })

	channelModelHealthCache.Lock()
	previousItems := channelModelHealthCache.items
	channelModelHealthCache.items = map[channelModelHealthKey]*ChannelModelHealth{
		normalizeChannelModelHealthKey(1, "default", "gpt-test"): {
			ChannelId:     1,
			Group:         "default",
			Model:         "gpt-test",
			State:         ChannelModelHealthOpen,
			CooldownUntil: 200,
		},
	}
	channelModelHealthCache.Unlock()
	t.Cleanup(func() {
		channelModelHealthCache.Lock()
		channelModelHealthCache.items = previousItems
		channelModelHealthCache.Unlock()
	})

	assert.Equal(t, []int{2}, filterChannelModelHealthCandidates([]int{1, 2}, "default", "gpt-test", 100))
	assert.Equal(t, []int{1, 2}, filterChannelModelHealthCandidates([]int{1, 2}, "default", "other-model", 100))
}

func TestTryAcquireChannelModelDoesNotCacheHealthyPair(t *testing.T) {
	setting := operation_setting.GetChannelModelHealthSetting()
	previous := *setting
	setting.Enabled = true
	setting.ExcludedChannelIds = nil
	setting.ExcludedModels = nil
	t.Cleanup(func() { *setting = previous })

	channelModelHealthCache.Lock()
	previousItems := channelModelHealthCache.items
	channelModelHealthCache.items = make(map[channelModelHealthKey]*ChannelModelHealth)
	channelModelHealthCache.Unlock()
	t.Cleanup(func() {
		channelModelHealthCache.Lock()
		channelModelHealthCache.items = previousItems
		channelModelHealthCache.Unlock()
	})

	allowed, probe := TryAcquireChannelModel(21, "default", "healthy-model", 100, DefaultChannelModelHealthConfig())

	assert.True(t, allowed)
	assert.False(t, probe)
	channelModelHealthCache.RLock()
	defer channelModelHealthCache.RUnlock()
	assert.Empty(t, channelModelHealthCache.items)
}

func TestChannelModelHealthFailureWindowResets(t *testing.T) {
	health := ChannelModelHealth{State: ChannelModelHealthClosed}
	config := DefaultChannelModelHealthConfig()
	config.FailureThreshold = 2
	config.FailureWindowSeconds = 10

	health.ObserveFailure(100, 503, "upstream_error", "temporarily unavailable", config)
	health.ObserveFailure(111, 503, "upstream_error", "temporarily unavailable", config)

	assert.Equal(t, ChannelModelHealthSuspect, health.State)
	assert.Equal(t, 1, health.FailureCount)
}

func TestChannelModelHealthCooldownSaturatesWithoutOverflow(t *testing.T) {
	health := ChannelModelHealth{
		State:     ChannelModelHealthHalfOpen,
		OpenCount: 1000,
	}
	config := DefaultChannelModelHealthConfig()
	config.CooldownSeconds = 60
	config.MaxCooldownSeconds = 1800

	health.ObserveFailure(100, 503, "upstream_error", "still unavailable", config)

	assert.Equal(t, ChannelModelHealthOpen, health.State)
	assert.Equal(t, int64(1900), health.CooldownUntil)
}

func TestChannelModelHealthCooldownIncrementsLinearlyUntilMaximum(t *testing.T) {
	config := DefaultChannelModelHealthConfig()
	config.CooldownSeconds = 20
	config.MaxCooldownSeconds = 120
	health := ChannelModelHealth{State: ChannelModelHealthHalfOpen}

	health.ObserveFailure(100, 503, "upstream_error", "still unavailable", config)
	assert.Equal(t, int64(120), health.CooldownUntil)

	health.State = ChannelModelHealthHalfOpen
	health.ObserveFailure(200, 503, "upstream_error", "still unavailable", config)
	assert.Equal(t, int64(240), health.CooldownUntil)

	health.State = ChannelModelHealthHalfOpen
	health.ObserveFailure(300, 503, "upstream_error", "still unavailable", config)
	assert.Equal(t, int64(360), health.CooldownUntil)

	health.State = ChannelModelHealthHalfOpen
	health.ObserveFailure(400, 503, "upstream_error", "still unavailable", config)
	assert.Equal(t, int64(480), health.CooldownUntil)

	health.State = ChannelModelHealthHalfOpen
	health.ObserveFailure(500, 503, "upstream_error", "still unavailable", config)
	assert.Equal(t, int64(600), health.CooldownUntil)

	health.State = ChannelModelHealthHalfOpen
	health.ObserveFailure(600, 503, "upstream_error", "still unavailable", config)
	assert.Equal(t, int64(720), health.CooldownUntil)
}

func TestChannelModelHealthSuccessClosesHalfOpenState(t *testing.T) {
	health := ChannelModelHealth{
		State:              ChannelModelHealthHalfOpen,
		HalfOpenLeaseUntil: 210,
		FailureCount:       3,
	}

	health.ObserveSuccess(205)

	assert.Equal(t, ChannelModelHealthClosed, health.State)
	assert.Zero(t, health.FailureCount)
	assert.Zero(t, health.CooldownUntil)
	assert.Zero(t, health.HalfOpenLeaseUntil)
}

func TestOpenChannelModelHealthOpensEveryConfiguredGroup(t *testing.T) {
	truncateTables(t)
	setting := operation_setting.GetChannelModelHealthSetting()
	previous := *setting
	setting.Enabled = true
	setting.ExcludedChannelIds = nil
	setting.ExcludedModels = nil
	setting.CooldownSeconds = 60
	setting.MaxCooldownSeconds = 1800
	t.Cleanup(func() { *setting = previous })

	require.NoError(t, DB.Create(&Channel{Id: 41, Name: "provider", Status: common.ChannelStatusEnabled}).Error)
	require.NoError(t, DB.Create(&[]Ability{
		{ChannelId: 41, Group: "alpha", Model: "gpt-test", Enabled: true},
		{ChannelId: 41, Group: "beta", Model: "gpt-test", Enabled: true},
		{ChannelId: 41, Group: "alpha", Model: "other-model", Enabled: true},
	}).Error)

	updated, err := OpenChannelModelHealth(41, "gpt-test", 100)

	require.NoError(t, err)
	assert.Equal(t, 2, updated)
	for _, group := range []string{"alpha", "beta"} {
		health := GetChannelModelHealth(41, group, "gpt-test")
		assert.Equal(t, ChannelModelHealthOpen, health.State)
		assert.Equal(t, int64(160), health.CooldownUntil)
	}
	assert.Equal(t, ChannelModelHealthClosed, GetChannelModelHealth(41, "alpha", "other-model").State)
}

func TestRecoverChannelModelHealthUsesSuccessTransitionForEveryConfiguredGroup(t *testing.T) {
	truncateTables(t)
	setting := operation_setting.GetChannelModelHealthSetting()
	previous := *setting
	setting.Enabled = true
	setting.ExcludedChannelIds = nil
	setting.ExcludedModels = nil
	t.Cleanup(func() { *setting = previous })

	require.NoError(t, DB.Create(&Channel{Id: 42, Name: "provider", Status: common.ChannelStatusEnabled}).Error)
	require.NoError(t, DB.Create(&[]Ability{
		{ChannelId: 42, Group: "alpha", Model: "gpt-test", Enabled: true},
		{ChannelId: 42, Group: "beta", Model: "gpt-test", Enabled: true},
	}).Error)
	require.NoError(t, DB.Create(&[]ChannelModelHealth{
		{ChannelId: 42, Group: "alpha", Model: "gpt-test", State: ChannelModelHealthOpen, FailureCount: 3, CooldownUntil: 200},
		{ChannelId: 42, Group: "beta", Model: "gpt-test", State: ChannelModelHealthSuspect, FailureCount: 1},
	}).Error)
	InitChannelModelHealthCache()

	updated, err := RecoverChannelModelHealth(42, "gpt-test", 300)

	require.NoError(t, err)
	assert.Equal(t, 2, updated)
	for _, group := range []string{"alpha", "beta"} {
		health := GetChannelModelHealth(42, group, "gpt-test")
		assert.Equal(t, ChannelModelHealthClosed, health.State)
		assert.Zero(t, health.FailureCount)
		assert.Equal(t, int64(300), health.LastSuccessAt)
		assert.Zero(t, health.CooldownUntil)
	}
}

func TestChannelModelHealthGroupUsesBoundedIndexedString(t *testing.T) {
	field, ok := reflect.TypeOf(ChannelModelHealth{}).FieldByName("Group")
	require.True(t, ok)
	assert.Contains(t, strings.ToLower(field.Tag.Get("gorm")), "type:varchar(255)")
}

func TestBuildChannelModelIndexUsesEnabledAbilities(t *testing.T) {
	priority := int64(10)
	channels := []*Channel{
		{Id: 1, Status: 1, Priority: &priority},
		{Id: 2, Status: 1, Priority: &priority},
	}
	abilities := []*Ability{
		{ChannelId: 1, Group: "default", Model: "gpt-test", Enabled: true},
		{ChannelId: 2, Group: "default", Model: "gpt-test", Enabled: false},
	}

	index := buildChannelModelIndex(channels, abilities)

	require.Contains(t, index, "default")
	assert.Equal(t, []int{1}, index["default"]["gpt-test"])
}

func TestShouldObserveChannelModelFailure(t *testing.T) {
	assert.True(t, ShouldObserveChannelModelFailure(testNewAPIError(503, "bad_response_status")))
	assert.True(t, ShouldObserveChannelModelFailure(testNewAPIError(404, "model_not_found")))
	assert.False(t, ShouldObserveChannelModelFailure(testNewAPIError(404, "bad_response_status_code")))
	assert.False(t, ShouldObserveChannelModelFailure(testNewAPIError(400, "invalid_request")))
	assert.False(t, ShouldObserveChannelModelFailure(testNewAPIError(401, "bad_response_status_code")))
	assert.False(t, ShouldObserveChannelModelFailure(testNewAPIError(402, "bad_response_status_code")))
	assert.False(t, ShouldObserveChannelModelFailure(testNewAPIError(403, "bad_response_status_code")))
}

func TestListChannelModelHealthJoinsChannelAndFilters(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Create(&Channel{Id: 11, Name: "cheap-provider", Status: 1}).Error)
	require.NoError(t, DB.Create(&Ability{
		ChannelId: 11,
		Group:     "default",
		Model:     "gpt-test",
		Enabled:   true,
	}).Error)
	require.NoError(t, DB.Create(&ChannelModelHealth{
		ChannelId: 11,
		Group:     "default",
		Model:     "gpt-test",
		State:     ChannelModelHealthOpen,
		UpdatedAt: 100,
	}).Error)

	items, total, err := ListChannelModelHealth(ChannelModelHealthListParams{
		State:    ChannelModelHealthOpen,
		Page:     1,
		PageSize: 20,
	})

	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, items, 1)
	assert.Equal(t, "cheap-provider", items[0].ChannelName)
	assert.Equal(t, "gpt-test", items[0].Model)
}

func TestListChannelModelHealthIncludesEnabledAbilityPairs(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Create(&[]Channel{
		{Id: 11, Name: "cheap-provider", Status: common.ChannelStatusEnabled},
		{Id: 12, Name: "disabled-provider", Status: common.ChannelStatusManuallyDisabled},
	}).Error)
	require.NoError(t, DB.Create(&[]Ability{
		{ChannelId: 11, Group: "default", Model: "healthy-model", Enabled: true},
		{ChannelId: 11, Group: "default", Model: "broken-model", Enabled: true},
		{ChannelId: 12, Group: "default", Model: "disabled-model", Enabled: false},
	}).Error)
	require.NoError(t, DB.Create(&ChannelModelHealth{
		ChannelId: 11,
		Group:     "default",
		Model:     "broken-model",
		State:     ChannelModelHealthSuspect,
	}).Error)

	items, total, err := ListChannelModelHealth(ChannelModelHealthListParams{
		Page:     1,
		PageSize: 20,
	})

	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	require.Len(t, items, 2)
	var healthy ChannelModelHealthView
	for _, item := range items {
		if item.Model == "healthy-model" {
			healthy = item
		}
	}
	assert.Equal(t, "cheap-provider", healthy.ChannelName)
	assert.Equal(t, ChannelModelHealthClosed, healthy.State)
	assert.False(t, healthy.HealthRecordExists)
}

func TestListChannelModelHealthIncludesLatestMatchingRequestIdentity(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Create(&Channel{
		Id:     11,
		Name:   "provider",
		Status: common.ChannelStatusEnabled,
	}).Error)
	require.NoError(t, DB.Create(&Ability{
		ChannelId: 11,
		Group:     "stable",
		Model:     "gpt-test",
		Enabled:   true,
	}).Error)
	require.NoError(t, LOG_DB.Create(&[]Log{
		{
			ChannelId: 11,
			Group:     "stable",
			ModelName: "gpt-test",
			Username:  "old-user",
			TokenName: "old-token",
			CreatedAt: 100,
			Type:      LogTypeConsume,
		},
		{
			ChannelId: 11,
			Group:     "stable",
			ModelName: "gpt-test",
			Username:  "latest-user",
			TokenName: "latest-token",
			CreatedAt: 200,
			Type:      LogTypeError,
		},
		{
			ChannelId: 11,
			Group:     "other-group",
			ModelName: "gpt-test",
			Username:  "wrong-group-user",
			TokenName: "wrong-group-token",
			CreatedAt: 300,
			Type:      LogTypeConsume,
		},
		{
			ChannelId: 11,
			Group:     "stable",
			ModelName: "other-model",
			Username:  "wrong-model-user",
			TokenName: "wrong-model-token",
			CreatedAt: 400,
			Type:      LogTypeConsume,
		},
	}).Error)

	items, total, err := ListChannelModelHealth(ChannelModelHealthListParams{
		ChannelId: 11,
		Page:      1,
		PageSize:  20,
	})

	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, items, 1)
	assert.Equal(t, "latest-user", items[0].LastRequestUsername)
	assert.Equal(t, "latest-token", items[0].LastRequestTokenName)
	assert.Equal(t, int64(200), items[0].LastRequestAt)
}

func TestGetChannelModelHealthSummarySeparatesWaitingProbeFromActiveCircuit(t *testing.T) {
	truncateTables(t)
	now := common.GetTimestamp()
	require.NoError(t, DB.Create(&Channel{Id: 11, Name: "provider", Status: common.ChannelStatusEnabled}).Error)
	require.NoError(t, DB.Create(&[]Ability{
		{ChannelId: 11, Group: "default", Model: "waiting-probe", Enabled: true},
		{ChannelId: 11, Group: "default", Model: "active-circuit", Enabled: true},
	}).Error)
	items := []ChannelModelHealth{
		{
			ChannelId:     11,
			Group:         "default",
			Model:         "waiting-probe",
			State:         ChannelModelHealthOpen,
			CooldownUntil: now - 1,
		},
		{
			ChannelId:     11,
			Group:         "default",
			Model:         "active-circuit",
			State:         ChannelModelHealthOpen,
			CooldownUntil: now + 3600,
		},
	}
	require.NoError(t, DB.Create(&items).Error)

	summary, err := GetChannelModelHealthSummary()

	require.NoError(t, err)
	require.Len(t, summary, 1)
	assert.Equal(t, int64(1), summary[0].Open)
	assert.Equal(t, int64(1), summary[0].Ready)
	assert.Empty(t, summary[0].Models)
}

func TestGetChannelModelHealthSummaryForChannelsLimitsModelsToRequestedChannels(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Create(&[]Channel{
		{Id: 21, Name: "first", Status: common.ChannelStatusEnabled},
		{Id: 22, Name: "second", Status: common.ChannelStatusEnabled},
	}).Error)
	require.NoError(t, DB.Create(&[]Ability{
		{ChannelId: 21, Group: "alpha", Model: "gpt-first", Enabled: true},
		{ChannelId: 22, Group: "beta", Model: "gpt-second", Enabled: true},
	}).Error)

	summary, err := GetChannelModelHealthSummaryForChannels([]int{22}, true)

	require.NoError(t, err)
	require.Len(t, summary, 1)
	assert.Equal(t, 22, summary[0].ChannelId)
	require.Len(t, summary[0].Models, 1)
	assert.Equal(t, "gpt-second", summary[0].Models[0].Model)
}

func TestListChannelModelHealthProbeCandidatesReturnsOnlyDueItems(t *testing.T) {
	truncateTables(t)
	now := common.GetTimestamp()
	items := []ChannelModelHealth{
		{
			ChannelId:     11,
			Group:         "default",
			Model:         "open-ready",
			State:         ChannelModelHealthOpen,
			CooldownUntil: now - 1,
		},
		{
			ChannelId:     12,
			Group:         "default",
			Model:         "open-cooling",
			State:         ChannelModelHealthOpen,
			CooldownUntil: now + 60,
		},
		{
			ChannelId:          13,
			Group:              "default",
			Model:              "half-open-ready",
			State:              ChannelModelHealthHalfOpen,
			HalfOpenLeaseUntil: now - 1,
		},
		{
			ChannelId:          14,
			Group:              "default",
			Model:              "half-open-busy",
			State:              ChannelModelHealthHalfOpen,
			HalfOpenLeaseUntil: now + 60,
		},
		{
			ChannelId:    15,
			Group:        "default",
			Model:        "suspect",
			State:        ChannelModelHealthSuspect,
			FailureCount: 1,
		},
	}
	require.NoError(t, DB.Create(&items).Error)

	candidates, err := ListChannelModelHealthProbeCandidates(now, 10)

	require.NoError(t, err)
	require.Len(t, candidates, 2)
	assert.Equal(t, 11, candidates[0].ChannelId)
	assert.Equal(t, 13, candidates[1].ChannelId)
}

func TestGetChannelModelHealthSummaryIncludesProblematicModels(t *testing.T) {
	truncateTables(t)
	now := common.GetTimestamp()
	require.NoError(t, DB.Create(&Channel{Id: 11, Name: "provider", Status: common.ChannelStatusEnabled}).Error)
	require.NoError(t, DB.Create(&Ability{
		ChannelId: 11,
		Group:     "stable",
		Model:     "healthy-model",
		Enabled:   true,
	}).Error)
	require.NoError(t, DB.Create(&[]Ability{
		{ChannelId: 11, Group: "stable", Model: "gpt-5.6-luna", Enabled: true},
		{ChannelId: 11, Group: "backup", Model: "claude-test", Enabled: true},
		{ChannelId: 11, Group: "stable", Model: "recovered-model", Enabled: true},
	}).Error)
	items := []ChannelModelHealth{
		{
			ChannelId:      11,
			Group:          "stable",
			Model:          "gpt-5.6-luna",
			State:          ChannelModelHealthOpen,
			CooldownUntil:  now + 3600,
			FailureCount:   3,
			LastStatusCode: 502,
			LastErrorCode:  "upstream_error",
			LastError:      "upstream unavailable",
			UpdatedAt:      now,
		},
		{
			ChannelId:      11,
			Group:          "backup",
			Model:          "claude-test",
			State:          ChannelModelHealthSuspect,
			FailureCount:   1,
			LastStatusCode: 404,
			LastErrorCode:  "model_not_found",
			LastError:      "model is unavailable",
			UpdatedAt:      now - 1,
		},
		{
			ChannelId: 11,
			Group:     "stable",
			Model:     "recovered-model",
			State:     ChannelModelHealthClosed,
		},
	}
	require.NoError(t, DB.Create(&items).Error)
	require.NoError(t, LOG_DB.Create(&Log{
		ChannelId: 11,
		Group:     "stable",
		ModelName: "gpt-5.6-luna",
		Username:  "latest-user",
		TokenName: "latest-token",
		CreatedAt: now,
		Type:      LogTypeError,
	}).Error)

	summary, err := GetChannelModelHealthSummaryForChannels([]int{11}, true)

	require.NoError(t, err)
	require.Len(t, summary, 1)
	assert.Equal(t, int64(4), summary[0].Total)
	assert.Equal(t, int64(1), summary[0].Healthy)
	assert.Equal(t, int64(1), summary[0].Closed)
	require.Len(t, summary[0].Issues, 2)
	require.Len(t, summary[0].Models, 4)
	modelsByName := make(map[string]ChannelModelHealthSummaryModel, len(summary[0].Models))
	for _, item := range summary[0].Models {
		modelsByName[item.Model] = item
	}
	assert.Equal(t, ChannelModelHealthClosed, modelsByName["healthy-model"].State)
	assert.False(t, modelsByName["healthy-model"].HealthRecordExists)
	assert.Equal(t, ChannelModelHealthOpen, modelsByName["gpt-5.6-luna"].State)
	assert.Equal(t, ChannelModelHealthClosed, modelsByName["recovered-model"].State)
	assert.True(t, modelsByName["recovered-model"].HealthRecordExists)
	assert.Equal(t, "gpt-5.6-luna", summary[0].Issues[0].Model)
	assert.Equal(t, "stable", summary[0].Issues[0].Group)
	assert.Equal(t, ChannelModelHealthOpen, summary[0].Issues[0].State)
	assert.False(t, summary[0].Issues[0].Ready)
	assert.Equal(t, 502, summary[0].Issues[0].LastStatusCode)
	assert.Equal(t, "latest-user", summary[0].Issues[0].LastRequestUsername)
	assert.Equal(t, "latest-token", summary[0].Issues[0].LastRequestTokenName)
	assert.Equal(t, now, summary[0].Issues[0].LastRequestAt)
	assert.Equal(t, "claude-test", summary[0].Issues[1].Model)
}

func TestGetChannelModelHealthSummaryExcludesRemovedModelHealthRecords(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Create(&Channel{
		Id:     11,
		Name:   "provider",
		Status: common.ChannelStatusEnabled,
	}).Error)
	require.NoError(t, DB.Create(&Ability{
		ChannelId: 11,
		Group:     "stable",
		Model:     "configured-model",
		Enabled:   true,
	}).Error)
	require.NoError(t, DB.Create(&ChannelModelHealth{
		ChannelId: 11,
		Group:     "stable",
		Model:     "removed-model",
		State:     ChannelModelHealthOpen,
	}).Error)

	summary, err := GetChannelModelHealthSummaryForChannels([]int{11}, true)

	require.NoError(t, err)
	require.Len(t, summary, 1)
	assert.Equal(t, int64(1), summary[0].Total)
	require.Len(t, summary[0].Models, 1)
	assert.Equal(t, "configured-model", summary[0].Models[0].Model)
	assert.Empty(t, summary[0].Issues)
}

func TestResetChannelModelHealthOnlyRemovesMatchingPair(t *testing.T) {
	truncateTables(t)
	items := []ChannelModelHealth{
		{ChannelId: 11, Group: "default", Model: "gpt-test", State: ChannelModelHealthOpen},
		{ChannelId: 11, Group: "default", Model: "other-model", State: ChannelModelHealthSuspect},
	}
	require.NoError(t, DB.Create(&items).Error)
	InitChannelModelHealthCache()

	require.NoError(t, ResetChannelModelHealth(11, "default", "gpt-test"))

	var remaining []ChannelModelHealth
	require.NoError(t, DB.Find(&remaining).Error)
	require.Len(t, remaining, 1)
	assert.Equal(t, "other-model", remaining[0].Model)
	assert.Equal(t, ChannelModelHealthClosed, GetChannelModelHealth(11, "default", "gpt-test").State)
}

func TestPersistChannelModelHealthUpsertsPair(t *testing.T) {
	truncateTables(t)
	health := &ChannelModelHealth{
		ChannelId: 11,
		Group:     "default",
		Model:     "gpt-test",
		State:     ChannelModelHealthSuspect,
	}
	PersistChannelModelHealth(health)
	health.State = ChannelModelHealthOpen
	health.FailureCount = 3
	PersistChannelModelHealth(health)

	var items []ChannelModelHealth
	require.NoError(t, DB.Find(&items).Error)
	require.Len(t, items, 1)
	assert.Equal(t, ChannelModelHealthOpen, items[0].State)
	assert.Equal(t, 3, items[0].FailureCount)
}
