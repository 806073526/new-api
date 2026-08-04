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
	assert.False(t, ShouldObserveChannelModelFailure(testNewAPIError(400, "invalid_request")))
	assert.False(t, ShouldObserveChannelModelFailure(testNewAPIError(401, "bad_response_status_code")))
	assert.False(t, ShouldObserveChannelModelFailure(testNewAPIError(402, "bad_response_status_code")))
	assert.False(t, ShouldObserveChannelModelFailure(testNewAPIError(403, "bad_response_status_code")))
}

func TestListChannelModelHealthJoinsChannelAndFilters(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Create(&Channel{Id: 11, Name: "cheap-provider", Status: 1}).Error)
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

func TestGetChannelModelHealthSummarySeparatesWaitingProbeFromActiveCircuit(t *testing.T) {
	truncateTables(t)
	now := common.GetTimestamp()
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
}

func TestGetChannelModelHealthSummaryIncludesProblematicModels(t *testing.T) {
	truncateTables(t)
	now := common.GetTimestamp()
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

	summary, err := GetChannelModelHealthSummary()

	require.NoError(t, err)
	require.Len(t, summary, 1)
	require.Len(t, summary[0].Issues, 2)
	assert.Equal(t, "gpt-5.6-luna", summary[0].Issues[0].Model)
	assert.Equal(t, "stable", summary[0].Issues[0].Group)
	assert.Equal(t, ChannelModelHealthOpen, summary[0].Issues[0].State)
	assert.False(t, summary[0].Issues[0].Ready)
	assert.Equal(t, 502, summary[0].Issues[0].LastStatusCode)
	assert.Equal(t, "claude-test", summary[0].Issues[1].Model)
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
