package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelModelManualDisableBlocksEveryGroupAndOnlyManualRecoveryRestoresIt(t *testing.T) {
	if DB == nil {
		t.Skip("database is not initialized")
	}
	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = true
	t.Cleanup(func() { common.MemoryCacheEnabled = previousMemoryCacheEnabled })
	channel := Channel{Name: "manual-disable-routing", Key: "sk-manual-disable-routing", Status: 1}
	require.NoError(t, DB.Create(&channel).Error)
	modelName := "gpt-manual-disable-routing"
	require.NoError(t, DB.Create(&[]Ability{
		{ChannelId: channel.Id, Group: "alpha", Model: modelName, Enabled: true},
		{ChannelId: channel.Id, Group: "beta", Model: modelName, Enabled: true},
	}).Error)

	require.NoError(t, DisableChannelModelManually(channel.Id, modelName, "bad streaming response", 7, "operator", 100))
	assert.True(t, IsChannelModelManuallyDisabled(channel.Id, modelName))
	assert.Empty(t, filterChannelModelManualDisableCandidates([]int{channel.Id}, "alpha", modelName))
	assert.Empty(t, filterChannelModelManualDisableCandidates([]int{channel.Id}, "beta", modelName))
	common.MemoryCacheEnabled = false
	assert.True(t, IsChannelModelManuallyDisabled(channel.Id, modelName))
	assert.Empty(t, filterChannelModelManualDisableCandidates([]int{channel.Id}, "alpha", modelName))
	common.MemoryCacheEnabled = true

	// A successful request or automatic health recovery must not release the manual lock.
	ObserveChannelModelSuccess(channel.Id, "alpha", modelName, 200)
	assert.True(t, IsChannelModelManuallyDisabled(channel.Id, modelName))

	require.NoError(t, RecoverChannelModelManuallyDisabled(channel.Id, modelName, 8, "operator", 300))
	assert.False(t, IsChannelModelManuallyDisabled(channel.Id, modelName))
	assert.Equal(t, []int{channel.Id}, filterChannelModelManualDisableCandidates([]int{channel.Id}, "alpha", modelName))

	var disable ChannelModelManualDisable
	require.NoError(t, DB.Where("channel_id = ? AND model = ?", channel.Id, modelName).First(&disable).Error)
	assert.False(t, disable.Active)
	assert.Equal(t, int64(300), disable.RecoveredAt)
}
