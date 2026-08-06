package operation_setting

import (
	"strings"

	"github.com/QuantumNous/new-api/setting/config"
)

type ChannelModelHealthSetting struct {
	Enabled                     bool     `json:"enabled"`
	FailureThreshold            int      `json:"failure_threshold"`
	FailureWindowSeconds        int64    `json:"failure_window_seconds"`
	FirstResponseTimeoutSeconds int64    `json:"first_response_timeout_seconds"`
	CooldownSeconds             int64    `json:"cooldown_seconds"`
	MaxCooldownSeconds          int64    `json:"max_cooldown_seconds"`
	HalfOpenLeaseSeconds        int64    `json:"half_open_lease_seconds"`
	ActiveProbeIntervalSeconds  int64    `json:"active_probe_interval_seconds"`
	ExcludedChannelIds          []int    `json:"excluded_channel_ids"`
	ExcludedModels              []string `json:"excluded_models"`
}

var channelModelHealthSetting = ChannelModelHealthSetting{
	Enabled:                     false,
	FailureThreshold:            3,
	FailureWindowSeconds:        60,
	FirstResponseTimeoutSeconds: 0,
	CooldownSeconds:             60,
	MaxCooldownSeconds:          1800,
	HalfOpenLeaseSeconds:        30,
	ActiveProbeIntervalSeconds:  0,
	ExcludedChannelIds:          []int{},
	ExcludedModels:              []string{},
}

func init() {
	config.GlobalConfig.Register("channel_model_health_setting", &channelModelHealthSetting)
}

func GetChannelModelHealthSetting() *ChannelModelHealthSetting {
	return &channelModelHealthSetting
}

func (setting *ChannelModelHealthSetting) IsChannelExcluded(channelId int) bool {
	if setting == nil {
		return false
	}
	for _, excludedId := range setting.ExcludedChannelIds {
		if excludedId == channelId {
			return true
		}
	}
	return false
}

func (setting *ChannelModelHealthSetting) IsModelExcluded(model string) bool {
	if setting == nil {
		return false
	}
	normalized := strings.TrimSpace(model)
	for _, excludedModel := range setting.ExcludedModels {
		if strings.TrimSpace(excludedModel) == normalized {
			return true
		}
	}
	return false
}
