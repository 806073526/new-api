package controller

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeChannelModelHealthListParamsUsesApiDefaults(t *testing.T) {
	params := model.ChannelModelHealthListParams{}

	normalizeChannelModelHealthListParams(&params)

	assert.Equal(t, 1, params.Page)
	assert.Equal(t, 50, params.PageSize)
}

func TestNormalizeChannelModelHealthListParamsCapsPageSize(t *testing.T) {
	params := model.ChannelModelHealthListParams{Page: 3, PageSize: 500}

	normalizeChannelModelHealthListParams(&params)

	assert.Equal(t, 3, params.Page)
	assert.Equal(t, 200, params.PageSize)
}

func TestParseChannelModelHealthSummaryChannelIDsDeduplicatesValidIDs(t *testing.T) {
	channelIDs, err := parseChannelModelHealthSummaryChannelIDs(" 7, 9,7 ")

	require.NoError(t, err)
	assert.Equal(t, []int{7, 9}, channelIDs)
}

func TestParseChannelModelHealthSummaryChannelIDsRejectsInvalidID(t *testing.T) {
	channelIDs, err := parseChannelModelHealthSummaryChannelIDs("7,zero")

	require.Error(t, err)
	assert.Nil(t, channelIDs)
}

func TestShouldObserveChannelModelTestRequiresExplicitModelFlag(t *testing.T) {
	gin.SetMode(gin.TestMode)

	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest(http.MethodGet, "/api/channel/test/7?model=gpt-test&observe_health=true", nil)
	assert.True(t, shouldObserveChannelModelTest(context, "gpt-test"))

	context, _ = gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest(http.MethodGet, "/api/channel/test/7?model=gpt-test", nil)
	assert.False(t, shouldObserveChannelModelTest(context, "gpt-test"))

	context, _ = gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest(http.MethodGet, "/api/channel/test/7?observe_health=true", nil)
	assert.False(t, shouldObserveChannelModelTest(context, ""))
}

func TestObserveChannelModelTestResultAppliesAnEligibleFailureToAllGroups(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	setting := operation_setting.GetChannelModelHealthSetting()
	previous := *setting
	setting.Enabled = true
	setting.FailureThreshold = 1
	setting.ExcludedChannelIds = nil
	setting.ExcludedModels = nil
	t.Cleanup(func() { *setting = previous })

	channel := &model.Channel{Id: 73, Name: "provider", Status: common.ChannelStatusEnabled}
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, db.Create(&[]model.Ability{
		{ChannelId: channel.Id, Group: "alpha", Model: "gpt-test", Enabled: true},
		{ChannelId: channel.Id, Group: "beta", Model: "gpt-test", Enabled: true},
	}).Error)
	require.NoError(t, model.ResetChannelModelHealth(channel.Id, "", "gpt-test"))

	observeChannelModelTestResult(channel, "gpt-test", testResult{
		newAPIError: types.NewErrorWithStatusCode(
			errors.New("upstream unavailable"),
			types.ErrorCodeBadResponseStatusCode,
			http.StatusServiceUnavailable,
		),
	}, 0)

	for _, group := range []string{"alpha", "beta"} {
		health := model.GetChannelModelHealth(channel.Id, group, "gpt-test")
		assert.Equal(t, model.ChannelModelHealthOpen, health.State)
	}
}
