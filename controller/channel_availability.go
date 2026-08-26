package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
)

func GetChannelAvailability(c *gin.Context) {
	userGroup, err := model.GetUserGroup(c.GetInt("id"), false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	usableGroups := service.GetUserUsableGroups(userGroup)
	groups := make([]string, 0, len(usableGroups))
	for group := range usableGroups {
		groups = append(groups, group)
	}
	snapshot, err := model.GetChannelAvailabilitySnapshotForGroups(common.GetTimestamp(), groups)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, snapshot)
}

func TestChannelAvailability(c *gin.Context) {
	task, created, err := service.EnqueueSystemTask(model.SystemTaskTypeChannelTest, channelTestTaskPayload{
		Mode:      operation_setting.ChannelTestModeScheduledAll,
		AllModels: true,
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !created {
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"message": "已有渠道测试任务正在运行或等待中",
			"data": gin.H{
				"task_id": task.TaskID,
				"status":  task.Status,
			},
		})
		return
	}
	common.ApiSuccess(c, gin.H{
		"task_id": task.TaskID,
		"status":  task.Status,
	})
}

func GetChannelAvailabilityTestStatus(c *gin.Context) {
	taskID := c.Query("task_id")
	var (
		task *model.SystemTask
		err  error
	)
	if taskID == "" {
		task, err = model.GetActiveSystemTask(model.SystemTaskTypeChannelTest)
	} else {
		task, err = model.GetSystemTaskByTaskID(taskID)
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if task == nil {
		common.ApiSuccess(c, nil)
		return
	}
	common.ApiSuccess(c, task.ToResponse())
}
