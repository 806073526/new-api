package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAggregateUpstreamHubBillingLogsSplitsRatiosAndSeparatesRefunds(t *testing.T) {
	items, err := AggregateUpstreamHubBillingLogs([]*Log{
		{
			CreatedAt: 1704067212,
			Type:      LogTypeConsume,
			Quota:     1400000,
			ChannelId: 12,
			Group:     "vip",
			ModelName: "gpt-4o",
			Other:     `{"group_ratio":1.4,"user_group_ratio":0}`,
		},
		{
			CreatedAt: 1704067240,
			Type:      LogTypeRefund,
			Quota:     140000,
			ChannelId: 12,
			Group:     "vip",
			ModelName: "gpt-4o",
			Other:     `{"group_ratio":1.4,"user_group_ratio":1.2}`,
		},
	}, 1704067200, 1704067500, 300)

	require.NoError(t, err)
	require.Len(t, items, 2)

	assert.Equal(t, int64(1704067200), items[0].BucketStart)
	assert.Equal(t, int64(1704067500), items[0].BucketEnd)
	assert.Equal(t, 12, items[0].ChannelID)
	assert.Equal(t, "vip", items[0].Group)
	assert.Equal(t, "gpt-4o", items[0].ModelName)
	assert.Equal(t, 1.2, items[0].EffectiveGroupRatio)
	assert.Equal(t, "user_group_ratio", items[0].RatioSource)
	assert.Equal(t, "exact", items[0].NormalizationStatus)
	assert.Equal(t, int64(0), items[0].ConsumeQuota)
	assert.Equal(t, int64(140000), items[0].RefundQuota)

	assert.Equal(t, 1.4, items[1].EffectiveGroupRatio)
	assert.Equal(t, "group_ratio", items[1].RatioSource)
	assert.Equal(t, "exact", items[1].NormalizationStatus)
	assert.Equal(t, int64(1400000), items[1].ConsumeQuota)
	assert.Equal(t, int64(0), items[1].RefundQuota)
}

func TestAggregateUpstreamHubBillingLogsMarksMissingRatioAsEstimated(t *testing.T) {
	items, err := AggregateUpstreamHubBillingLogs([]*Log{
		{
			CreatedAt: 1704067212,
			Type:      LogTypeConsume,
			Quota:     500000,
			ChannelId: 7,
			Group:     "default",
			ModelName: "gpt-4o-mini",
		},
		{
			CreatedAt: 1704067214,
			Type:      LogTypeError,
			Quota:     999999,
			ChannelId: 7,
			Group:     "default",
			ModelName: "gpt-4o-mini",
		},
	}, 1704067200, 1704067500, 300)

	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, 1.0, items[0].EffectiveGroupRatio)
	assert.Equal(t, "default", items[0].RatioSource)
	assert.Equal(t, "estimated", items[0].NormalizationStatus)
	assert.Equal(t, int64(500000), items[0].ConsumeQuota)
	assert.Equal(t, int64(0), items[0].RefundQuota)
}

func TestAggregateUpstreamHubBillingLogsMarksZeroGroupRatioAsUnavailable(t *testing.T) {
	items, err := AggregateUpstreamHubBillingLogs([]*Log{
		{
			CreatedAt: 1704067212,
			Type:      LogTypeConsume,
			Quota:     500000,
			ChannelId: 7,
			Group:     "free",
			ModelName: "gpt-4o-mini",
			Other:     `{"group_ratio":0}`,
		},
	}, 1704067200, 1704067500, 300)

	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, 0.0, items[0].EffectiveGroupRatio)
	assert.Equal(t, "group_ratio", items[0].RatioSource)
	assert.Equal(t, "unavailable", items[0].NormalizationStatus)
	assert.Equal(t, int64(500000), items[0].ConsumeQuota)
}
