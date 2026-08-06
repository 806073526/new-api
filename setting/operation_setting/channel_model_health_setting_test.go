package operation_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestChannelModelHealthSettingExclusions(t *testing.T) {
	setting := ChannelModelHealthSetting{
		ExcludedChannelIds: []int{2, 8},
		ExcludedModels:     []string{"gpt-test", "claude-test"},
	}

	assert.True(t, setting.IsChannelExcluded(8))
	assert.False(t, setting.IsChannelExcluded(9))
	assert.True(t, setting.IsModelExcluded(" gpt-test "))
	assert.False(t, setting.IsModelExcluded("gpt-other"))
}

func TestChannelModelHealthDefaultsDisableActiveProbe(t *testing.T) {
	assert.Zero(t, channelModelHealthSetting.ActiveProbeIntervalSeconds)
}
