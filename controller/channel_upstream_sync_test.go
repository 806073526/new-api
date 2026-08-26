package controller

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildUpstreamPriorityPlanUsesLowestRatioAnd500Step10(t *testing.T) {
	plan := buildUpstreamPriorityPlan([]model.ChannelUpstreamMetric{
		{ChannelId: 1, UpstreamRatio: 0.8, UpdatedAt: time.Now()},
		{ChannelId: 2, UpstreamRatio: 0.5, UpdatedAt: time.Now()},
		{ChannelId: 3, UpstreamRatio: 0.5, UpdatedAt: time.Now()},
	}, nil, 500, 10)
	want := []upstreamHubPriorityInput{
		{ChannelID: 2, Priority: 500},
		{ChannelID: 3, Priority: 500},
		{ChannelID: 1, Priority: 490},
	}
	require.Len(t, plan, len(want))
	assert.Equal(t, want, plan)
}

func TestShouldAutoDisableUpstreamRatioUsesLocalGroupsAndStrictThreshold(t *testing.T) {
	tests := []struct {
		name          string
		groups        string
		upstreamRatio float64
		warningRatios map[string]float64
		autoDisable   map[string]bool
		want          bool
	}{
		{
			name:          "disables when a local group exceeds its warning ratio",
			groups:        "stable, vip",
			upstreamRatio: 1.6,
			warningRatios: map[string]float64{"stable": 1.5},
			autoDisable:   map[string]bool{"stable": true},
			want:          true,
		},
		{
			name:          "does not disable at the warning ratio",
			groups:        "stable",
			upstreamRatio: 1.5,
			warningRatios: map[string]float64{"stable": 1.5},
			autoDisable:   map[string]bool{"stable": true},
			want:          false,
		},
		{
			name:          "does not disable when the switch is off",
			groups:        "stable",
			upstreamRatio: 1.6,
			warningRatios: map[string]float64{"stable": 1.5},
			autoDisable:   map[string]bool{"stable": false},
			want:          false,
		},
		{
			name:          "does not disable without a positive warning ratio",
			groups:        "stable",
			upstreamRatio: 1.6,
			warningRatios: map[string]float64{"stable": 0},
			autoDisable:   map[string]bool{"stable": true},
			want:          false,
		},
		{
			name:          "disables when any local group matches",
			groups:        "default, stable",
			upstreamRatio: 2.1,
			warningRatios: map[string]float64{"stable": 2},
			autoDisable:   map[string]bool{"stable": true},
			want:          true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			channel := &model.Channel{Group: tt.groups}
			assert.Equal(t, tt.want, shouldAutoDisableUpstreamRatio(channel, tt.upstreamRatio, tt.warningRatios, tt.autoDisable))
		})
	}
}
