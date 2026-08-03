package middleware

import (
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTryAcquirePreferredChannelModelHonorsCircuitState(t *testing.T) {
	setting := operation_setting.GetChannelModelHealthSetting()
	previous := *setting
	setting.Enabled = true
	setting.FailureThreshold = 1
	setting.CooldownSeconds = 60
	setting.HalfOpenLeaseSeconds = 30
	setting.ExcludedChannelIds = nil
	setting.ExcludedModels = nil
	t.Cleanup(func() { *setting = previous })

	const channelID = 987654
	const group = "default"
	const modelName = "health-affinity-test"
	require.NoError(t, model.ResetChannelModelHealth(channelID, group, modelName))
	t.Cleanup(func() {
		_ = model.ResetChannelModelHealth(channelID, group, modelName)
	})

	upstreamErr := types.NewErrorWithStatusCode(
		errors.New("upstream unavailable"),
		types.ErrorCodeBadResponseStatusCode,
		503,
	)
	model.ObserveChannelModelFailure(channelID, group, modelName, upstreamErr, 100, model.GetChannelModelHealthConfig())

	allowed, probe := tryAcquirePreferredChannelModel(channelID, group, modelName, 159)
	assert.False(t, allowed)
	assert.False(t, probe)

	allowed, probe = tryAcquirePreferredChannelModel(channelID, group, modelName, 160)
	assert.True(t, allowed)
	assert.True(t, probe)

	allowed, probe = tryAcquirePreferredChannelModel(channelID, group, modelName, 160)
	assert.False(t, allowed)
	assert.False(t, probe)
}
