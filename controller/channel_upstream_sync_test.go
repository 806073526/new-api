package controller

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
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
	if len(plan) != len(want) {
		t.Fatalf("plan length = %d, want %d: %#v", len(plan), len(want), plan)
	}
	for i := range want {
		if plan[i] != want[i] {
			t.Fatalf("plan[%d] = %#v, want %#v", i, plan[i], want[i])
		}
	}
}
