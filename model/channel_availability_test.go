package model

import (
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestEvaluateChannelAvailabilityReturnsAvailableForFreshSuccess(t *testing.T) {
	state := evaluateChannelAvailability([]channelAvailabilityCandidate{
		{
			ChannelStatus: common.ChannelStatusEnabled,
			Health: &ChannelModelHealth{
				State:         ChannelModelHealthClosed,
				LastSuccessAt: 900,
			},
		},
	}, 1000, 300)

	assert.Equal(t, ChannelAvailabilityAvailable, state)
}

func TestEvaluateChannelAvailabilityDetailsIncludesFreshObservationMetadata(t *testing.T) {
	evaluation := evaluateChannelAvailabilityDetails([]channelAvailabilityCandidate{
		{
			ChannelStatus: common.ChannelStatusEnabled,
			Health: &ChannelModelHealth{
				State:                 ChannelModelHealthClosed,
				LastSuccessAt:         900,
				LastLatencyMs:         820,
				LastObservationSource: ChannelAvailabilitySourceActiveTest,
			},
		},
	}, 1000, 300)

	assert.Equal(t, ChannelAvailabilityAvailable, evaluation.State)
	assert.Equal(t, ChannelAvailabilityReasonFresh, evaluation.Reason)
	assert.Equal(t, int64(820), evaluation.LatencyMs)
	assert.Equal(t, ChannelAvailabilitySourceActiveTest, evaluation.Source)
}

func TestEvaluateChannelAvailabilityReturnsUnknownWithoutFreshEvidence(t *testing.T) {
	state := evaluateChannelAvailability([]channelAvailabilityCandidate{
		{
			ChannelStatus: common.ChannelStatusEnabled,
			Health: &ChannelModelHealth{
				State:         ChannelModelHealthClosed,
				LastSuccessAt: 600,
			},
		},
	}, 1000, 300)

	assert.Equal(t, ChannelAvailabilityUnknown, state)
}

func TestEvaluateChannelAvailabilityDetailsExplainsMissingAndExpiredEvidence(t *testing.T) {
	noObservation := evaluateChannelAvailabilityDetails([]channelAvailabilityCandidate{
		{
			ChannelStatus: common.ChannelStatusEnabled,
			Health:        &ChannelModelHealth{State: ChannelModelHealthClosed},
		},
	}, 1000, 300)
	assert.Equal(t, ChannelAvailabilityReasonNoObservation, noObservation.Reason)

	expired := evaluateChannelAvailabilityDetails([]channelAvailabilityCandidate{
		{
			ChannelStatus: common.ChannelStatusEnabled,
			Health: &ChannelModelHealth{
				State:                 ChannelModelHealthClosed,
				LastSuccessAt:         600,
				LastLatencyMs:         1200,
				LastObservationSource: ChannelAvailabilitySourceRequest,
			},
		},
	}, 1000, 300)
	assert.Equal(t, ChannelAvailabilityReasonStale, expired.Reason)
	assert.Equal(t, int64(1200), expired.LatencyMs)
	assert.Equal(t, ChannelAvailabilitySourceRequest, expired.Source)
}

func TestEvaluateChannelAvailabilityDetailsLabelsRecentFailure(t *testing.T) {
	evaluation := evaluateChannelAvailabilityDetails([]channelAvailabilityCandidate{
		{
			ChannelStatus: common.ChannelStatusEnabled,
			Health: &ChannelModelHealth{
				State:                 ChannelModelHealthSuspect,
				LastFailureAt:         950,
				LastLatencyMs:         1600,
				LastObservationSource: ChannelAvailabilitySourceRequest,
			},
		},
	}, 1000, 300)

	assert.Equal(t, ChannelAvailabilityUnknown, evaluation.State)
	assert.Equal(t, ChannelAvailabilityReasonRecentFailure, evaluation.Reason)
}

func TestEvaluateChannelAvailabilityReturnsUnavailableWhenAllCandidatesAreBlocked(t *testing.T) {
	state := evaluateChannelAvailability([]channelAvailabilityCandidate{
		{ChannelStatus: common.ChannelStatusManuallyDisabled},
		{
			ChannelStatus: common.ChannelStatusEnabled,
			Health: &ChannelModelHealth{
				State:         ChannelModelHealthOpen,
				CooldownUntil: 1200,
			},
		},
	}, 1000, 300)

	assert.Equal(t, ChannelAvailabilityUnavailable, state)
}

func TestChannelAvailabilityFreshnessUsesTwiceMonitorIntervalAndTimeout(t *testing.T) {
	monitor := operation_setting.GetMonitorSetting()
	previousMonitor := *monitor
	health := operation_setting.GetChannelModelHealthSetting()
	previousHealth := *health
	t.Cleanup(func() {
		*monitor = previousMonitor
		*health = previousHealth
	})

	monitor.AutoTestChannelMinutes = 2
	health.FirstResponseTimeoutSeconds = 7

	assert.Equal(t, int64(300), ChannelAvailabilityFreshnessSeconds())

	monitor.AutoTestChannelMinutes = 10
	assert.Equal(t, int64(1207), ChannelAvailabilityFreshnessSeconds())
}

func TestObserveChannelModelSuccessCreatesEvidenceWhenHealthCircuitIsDisabled(t *testing.T) {
	setting := operation_setting.GetChannelModelHealthSetting()
	previousSetting := *setting
	setting.Enabled = false
	setting.ExcludedChannelIds = nil
	setting.ExcludedModels = nil
	t.Cleanup(func() { *setting = previousSetting })

	previousDB := DB
	DB = nil
	t.Cleanup(func() { DB = previousDB })

	channelModelHealthCache.Lock()
	previousItems := channelModelHealthCache.items
	channelModelHealthCache.items = make(map[channelModelHealthKey]*ChannelModelHealth)
	channelModelHealthCache.Unlock()
	t.Cleanup(func() {
		channelModelHealthCache.Lock()
		channelModelHealthCache.items = previousItems
		channelModelHealthCache.Unlock()
	})

	ObserveChannelModelSuccessWithMetadata(7, "default", "gpt-test", 1000, 820, ChannelAvailabilitySourceRequest)

	health := GetChannelModelHealth(7, "default", "gpt-test")
	assert.Equal(t, int64(1000), health.LastSuccessAt)
	assert.Equal(t, int64(820), health.LastLatencyMs)
	assert.Equal(t, ChannelAvailabilitySourceRequest, health.LastObservationSource)
}

func TestGetChannelModelTestTargetsReturnsEnabledAbilitiesInStableOrder(t *testing.T) {
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	previousDB := DB
	DB = db
	t.Cleanup(func() {
		DB = previousDB
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	require.NoError(t, db.AutoMigrate(&Channel{}, &Ability{}))
	require.NoError(t, db.Create(&[]Channel{
		{Id: 2, Status: common.ChannelStatusEnabled},
		{Id: 1, Status: common.ChannelStatusEnabled},
	}).Error)
	require.NoError(t, db.Create(&[]Ability{
		{ChannelId: 2, Group: "vip", Model: "z-model", Enabled: true},
		{ChannelId: 1, Group: "default", Model: "b-model", Enabled: true},
		{ChannelId: 1, Group: "default", Model: "disabled-model", Enabled: false},
	}).Error)

	targets, err := GetChannelModelTestTargets()
	require.NoError(t, err)
	assert.Equal(t, []ChannelModelTestTarget{
		{ChannelId: 1, Group: "default", Model: "b-model"},
		{ChannelId: 2, Group: "vip", Model: "z-model"},
	}, targets)
}
