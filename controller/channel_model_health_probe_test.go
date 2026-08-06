package controller

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
)

func TestChannelModelHealthProbeHandlerUsesConfiguredInterval(t *testing.T) {
	setting := operation_setting.GetChannelModelHealthSetting()
	previous := *setting
	t.Cleanup(func() { *setting = previous })

	setting.Enabled = true
	setting.ActiveProbeIntervalSeconds = 17
	handler := channelModelHealthProbeHandler{}

	assert.Equal(t, model.SystemTaskTypeChannelModelProbe, handler.Type())
	assert.True(t, handler.Enabled())
	assert.Equal(t, 17*time.Second, handler.Interval())

	setting.ActiveProbeIntervalSeconds = 0
	assert.False(t, handler.Enabled())
}
