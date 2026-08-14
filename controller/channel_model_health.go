package controller

import (
	"errors"
	"io"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func GetChannelModelHealth(c *gin.Context) {
	params := model.ChannelModelHealthListParams{
		Group: c.Query("group"),
		Model: c.Query("model"),
		State: model.ChannelModelHealthState(c.Query("state")),
	}
	params.ChannelId, _ = strconv.Atoi(c.Query("channel_id"))
	params.Page, _ = strconv.Atoi(c.Query("p"))
	params.PageSize, _ = strconv.Atoi(c.Query("page_size"))
	normalizeChannelModelHealthListParams(&params)
	items, total, err := model.ListChannelModelHealth(params)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"items":     items,
		"total":     total,
		"page":      params.Page,
		"page_size": params.PageSize,
	})
}

func GetChannelHealth(c *gin.Context) {
	channelID, err := strconv.Atoi(c.Param("id"))
	if err != nil || channelID <= 0 {
		common.ApiErrorMsg(c, "invalid channel id")
		return
	}
	params := model.ChannelModelHealthListParams{
		ChannelId: channelID,
		Group:     c.Query("group"),
		Model:     c.Query("model"),
		State:     model.ChannelModelHealthState(c.Query("state")),
	}
	params.Page, _ = strconv.Atoi(c.Query("p"))
	params.PageSize, _ = strconv.Atoi(c.Query("page_size"))
	normalizeChannelModelHealthListParams(&params)
	items, total, err := model.ListChannelModelHealth(params)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"items":     items,
		"total":     total,
		"page":      params.Page,
		"page_size": params.PageSize,
	})
}

func GetChannelModelHealthSummary(c *gin.Context) {
	channelIDs, err := parseChannelModelHealthSummaryChannelIDs(c.Query("channel_ids"))
	if err != nil {
		common.ApiErrorMsg(c, "invalid channel ids")
		return
	}
	var items []model.ChannelModelHealthSummaryItem
	if len(channelIDs) == 0 {
		items, err = model.GetChannelModelHealthSummary()
	} else {
		items, err = model.GetChannelModelHealthSummaryForChannels(channelIDs, true)
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, items)
}

func parseChannelModelHealthSummaryChannelIDs(value string) ([]int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	parts := strings.Split(value, ",")
	channelIDs := make([]int, 0, len(parts))
	seen := make(map[int]struct{}, len(parts))
	for _, part := range parts {
		channelID, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || channelID <= 0 {
			return nil, errors.New("invalid channel id")
		}
		if _, exists := seen[channelID]; exists {
			continue
		}
		seen[channelID] = struct{}{}
		channelIDs = append(channelIDs, channelID)
	}
	return channelIDs, nil
}

func GetChannelActivitySummary(c *gin.Context) {
	items, err := model.GetChannelActivitySummary(common.GetTimestamp())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, items)
}

func ResetChannelModelHealth(c *gin.Context) {
	var request struct {
		ChannelId int    `json:"channel_id"`
		Group     string `json:"group"`
		Model     string `json:"model"`
	}
	if err := c.ShouldBindJSON(&request); err != nil && !errors.Is(err, io.EOF) {
		common.ApiError(c, err)
		return
	}
	if err := model.ResetChannelModelHealth(request.ChannelId, request.Group, request.Model); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"reset": true})
}

type channelModelHealthActionRequest struct {
	ChannelId int    `json:"channel_id"`
	Model     string `json:"model"`
}

func OpenChannelModelHealth(c *gin.Context) {
	request, ok := bindChannelModelHealthActionRequest(c)
	if !ok {
		return
	}
	updated, err := model.OpenChannelModelHealth(request.ChannelId, request.Model, common.GetTimestamp())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"updated": updated})
}

func RecoverChannelModelHealth(c *gin.Context) {
	request, ok := bindChannelModelHealthActionRequest(c)
	if !ok {
		return
	}
	updated, err := model.RecoverChannelModelHealth(request.ChannelId, request.Model, common.GetTimestamp())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"updated": updated})
}

func DisableChannelModelManually(c *gin.Context) {
	request, ok := bindChannelModelManualDisableRequest(c)
	if !ok {
		return
	}
	if err := model.DisableChannelModelManually(
		request.ChannelId,
		request.Model,
		request.Reason,
		c.GetInt("id"),
		c.GetString("username"),
		common.GetTimestamp(),
	); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"disabled": true})
}

func RecoverChannelModelManuallyDisabled(c *gin.Context) {
	request, ok := bindChannelModelHealthActionRequest(c)
	if !ok {
		return
	}
	if err := model.RecoverChannelModelManuallyDisabled(
		request.ChannelId,
		request.Model,
		c.GetInt("id"),
		c.GetString("username"),
		common.GetTimestamp(),
	); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"recovered": true})
}

func bindChannelModelHealthActionRequest(c *gin.Context) (channelModelHealthActionRequest, bool) {
	var request channelModelHealthActionRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiError(c, err)
		return channelModelHealthActionRequest{}, false
	}
	request.Model = strings.TrimSpace(request.Model)
	if request.ChannelId <= 0 || request.Model == "" {
		common.ApiErrorMsg(c, "invalid channel model health request")
		return channelModelHealthActionRequest{}, false
	}
	return request, true
}

type channelModelManualDisableRequest struct {
	channelModelHealthActionRequest
	Reason string `json:"reason"`
}

func bindChannelModelManualDisableRequest(c *gin.Context) (channelModelManualDisableRequest, bool) {
	var request channelModelManualDisableRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiError(c, err)
		return channelModelManualDisableRequest{}, false
	}
	request.Model = strings.TrimSpace(request.Model)
	request.Reason = strings.TrimSpace(request.Reason)
	if request.ChannelId <= 0 || request.Model == "" {
		common.ApiErrorMsg(c, "invalid channel model manual disable request")
		return channelModelManualDisableRequest{}, false
	}
	return request, true
}

func normalizeChannelModelHealthListParams(params *model.ChannelModelHealthListParams) {
	if params.Page < 1 {
		params.Page = 1
	}
	if params.PageSize <= 0 {
		params.PageSize = 50
	}
	if params.PageSize > 200 {
		params.PageSize = 200
	}
}
