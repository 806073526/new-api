package controller

import (
	"errors"
	"io"
	"strconv"

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
	items, err := model.GetChannelModelHealthSummary()
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
