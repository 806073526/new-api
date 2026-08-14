package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

// RegisterScheduledSystemTasks wires periodic channel tests, model recovery
// probes, upstream model updates, and async task polling (Midjourney / Suno /
// video) into the system task framework so a DB lease dedups execution across
// multiple master instances and each run is recorded as one task row. Call this
// before service.StartSystemTaskRunner.
func RegisterScheduledSystemTasks() {
	service.RegisterSystemTaskHandler(channelTestHandler{})
	service.RegisterSystemTaskHandler(channelModelHealthProbeHandler{})
	service.RegisterSystemTaskHandler(modelUpdateHandler{})
	service.RegisterSystemTaskHandler(midjourneyPollHandler{})
	service.RegisterSystemTaskHandler(asyncTaskPollHandler{})
}

// channelModelHealthProbeHandler runs recovery probes for model-level health
// records whose cooldown or half-open lease has expired. It deliberately does
// not change channel-level status and does not record normal consume logs.
type channelModelHealthProbeHandler struct{}

func (channelModelHealthProbeHandler) Type() string { return model.SystemTaskTypeChannelModelProbe }

func (channelModelHealthProbeHandler) Enabled() bool {
	setting := operation_setting.GetChannelModelHealthSetting()
	return setting != nil && setting.Enabled && setting.ActiveProbeIntervalSeconds > 0
}

func (channelModelHealthProbeHandler) Interval() time.Duration {
	seconds := operation_setting.GetChannelModelHealthSetting().ActiveProbeIntervalSeconds
	if seconds <= 0 {
		return time.Minute
	}
	return time.Duration(seconds) * time.Second
}

func (channelModelHealthProbeHandler) NewPayload() any { return nil }

type channelModelHealthProbeSummary struct {
	Candidates int `json:"candidates"`
	Probed     int `json:"probed"`
	Succeeded  int `json:"succeeded"`
	Failed     int `json:"failed"`
	Skipped    int `json:"skipped"`
}

const channelModelHealthProbeBatchSize = 100

func (channelModelHealthProbeHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	summary, err := runChannelModelHealthProbeTask(ctx)
	if err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, summary, err)
		return
	}
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, summary, nil)
}

func runChannelModelHealthProbeTask(ctx context.Context) (channelModelHealthProbeSummary, error) {
	summary := channelModelHealthProbeSummary{}
	if ctx == nil {
		ctx = context.Background()
	}
	setting := operation_setting.GetChannelModelHealthSetting()
	if setting == nil || !setting.Enabled || setting.ActiveProbeIntervalSeconds <= 0 {
		return summary, nil
	}

	candidates, err := model.ListChannelModelHealthProbeCandidates(common.GetTimestamp(), channelModelHealthProbeBatchSize)
	if err != nil {
		return summary, err
	}
	summary.Candidates = len(candidates)
	if len(candidates) == 0 {
		return summary, nil
	}
	testUserID, err := resolveChannelTestUserID(nil)
	if err != nil {
		return summary, err
	}
	config := model.GetChannelModelHealthConfig()

	for _, candidate := range candidates {
		if err := ctx.Err(); err != nil {
			return summary, err
		}
		if model.IsChannelModelManuallyDisabled(candidate.ChannelId, candidate.Model) ||
			model.IsChannelModelHealthExcluded(candidate.ChannelId, candidate.Model) ||
			!model.IsChannelEnabledForGroupModel(candidate.Group, candidate.Model, candidate.ChannelId) {
			summary.Skipped++
			continue
		}

		channel, err := model.CacheGetChannel(candidate.ChannelId)
		if err != nil {
			channel, err = model.GetChannelById(candidate.ChannelId, true)
		}
		if err != nil || channel == nil || channel.Status != common.ChannelStatusEnabled {
			summary.Skipped++
			continue
		}

		allowed, probe := model.TryAcquireChannelModel(candidate.ChannelId, candidate.Group, candidate.Model, common.GetTimestamp(), config)
		if !allowed || !probe {
			summary.Skipped++
			continue
		}

		probeTimeout := channelModelHealthProbeTimeout(config)
		probeCtx, cancel := context.WithTimeout(ctx, probeTimeout)
		result := testChannelWithOptions(
			probeCtx,
			channel,
			testUserID,
			candidate.Model,
			"",
			shouldUseStreamForAutomaticChannelTest(channel),
			channelTestOptions{recordConsumeLog: false, group: candidate.Group},
		)
		cancel()
		if err := ctx.Err(); err != nil {
			return summary, err
		}
		summary.Probed++
		if result.localErr == nil && result.newAPIError == nil {
			model.ObserveChannelModelSuccess(candidate.ChannelId, candidate.Group, candidate.Model, common.GetTimestamp())
			summary.Succeeded++
			continue
		}
		if result.newAPIError != nil && model.ShouldObserveChannelModelFailure(result.newAPIError) {
			model.ObserveChannelModelFailure(candidate.ChannelId, candidate.Group, candidate.Model, result.newAPIError, common.GetTimestamp(), config)
			summary.Failed++
			continue
		}
		summary.Skipped++
	}
	return summary, nil
}

func channelModelHealthProbeTimeout(config model.ChannelModelHealthConfig) time.Duration {
	if config.FirstResponseTimeoutSeconds > 0 {
		return time.Duration(config.FirstResponseTimeoutSeconds) * time.Second
	}
	if config.HalfOpenLeaseSeconds > 0 {
		return time.Duration(config.HalfOpenLeaseSeconds) * time.Second
	}
	return 30 * time.Second
}

// channelTestHandler runs the scheduled "test all channels" job. Enablement and
// cadence still come from the monitor settings; only the execution path moved
// into the system task runner.
type channelTestHandler struct{}

func (channelTestHandler) Type() string { return model.SystemTaskTypeChannelTest }

func (channelTestHandler) Enabled() bool {
	return operation_setting.GetMonitorSetting().AutoTestChannelEnabled
}

func (channelTestHandler) Interval() time.Duration {
	minutes := operation_setting.GetMonitorSetting().AutoTestChannelMinutes
	if minutes <= 0 {
		minutes = 10
	}
	return time.Duration(minutes * float64(time.Minute))
}

func (channelTestHandler) NewPayload() any { return nil }

// channelTestTaskPayload controls one channel_test run. A nil/empty payload is a
// scheduled run, which uses the configured monitor ChannelTestMode and does not
// notify. A manual "test all channels" trigger sets Mode=scheduled_all and
// Notify=true to reproduce the legacy manual behavior (test every channel and
// notify root on completion).
type channelTestTaskPayload struct {
	Mode   string `json:"mode,omitempty"`
	Notify bool   `json:"notify,omitempty"`
}

func (channelTestHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	payload := channelTestTaskPayload{}
	if err := task.DecodePayload(&payload); err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, nil, err)
		return
	}
	summary, err := runChannelTestTask(ctx, payload.Mode, payload.Notify, service.NewSystemTaskProgressReporter(task, runnerID))
	if err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, nil, err)
		return
	}
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, summary, nil)
}

// modelUpdateHandler runs the scheduled upstream model update detection job.
type modelUpdateHandler struct{}

func (modelUpdateHandler) Type() string { return model.SystemTaskTypeModelUpdate }

func (modelUpdateHandler) Enabled() bool {
	return common.GetEnvOrDefaultBool("CHANNEL_UPSTREAM_MODEL_UPDATE_TASK_ENABLED", true)
}

func (modelUpdateHandler) Interval() time.Duration {
	intervalMinutes := common.GetEnvOrDefault(
		"CHANNEL_UPSTREAM_MODEL_UPDATE_TASK_INTERVAL_MINUTES",
		channelUpstreamModelUpdateTaskDefaultIntervalMinutes,
	)
	if intervalMinutes < 1 {
		intervalMinutes = channelUpstreamModelUpdateTaskDefaultIntervalMinutes
	}
	return time.Duration(intervalMinutes) * time.Minute
}

func (modelUpdateHandler) NewPayload() any { return nil }

// modelUpdateTaskPayload controls one model_update run. A scheduled run
// (Manual=false) respects the per-channel minimum check interval and may
// auto-apply detected models when a channel has auto-sync enabled. A manual
// "detect all" trigger sets Manual=true to reproduce the legacy detect-all
// semantics: force a re-check regardless of the interval and never auto-apply,
// so the admin reviews and applies changes explicitly.
type modelUpdateTaskPayload struct {
	Manual bool `json:"manual,omitempty"`
}

func (modelUpdateHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	payload := modelUpdateTaskPayload{}
	if err := task.DecodePayload(&payload); err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, nil, err)
		return
	}
	summary := runChannelUpstreamModelUpdateTaskOnce(ctx, payload.Manual, !payload.Manual, service.NewSystemTaskProgressReporter(task, runnerID))
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, summary, nil)
}

// midjourneyPollHandler runs one Midjourney polling pass per scheduled run.
// Enabled() folds the "are there unfinished tasks?" check into enablement so the
// scheduler creates no row when the system is idle; only when at least one
// Midjourney task is in progress does a row get scheduled.
type midjourneyPollHandler struct{}

func (midjourneyPollHandler) Type() string { return model.SystemTaskTypeMidjourneyPoll }

func (midjourneyPollHandler) Enabled() bool {
	return constant.UpdateTask && model.HasUnfinishedMidjourneyTasks()
}

func (midjourneyPollHandler) Interval() time.Duration { return 15 * time.Second }

func (midjourneyPollHandler) NewPayload() any { return nil }

func (midjourneyPollHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	summary := runMidjourneyTaskUpdateOnce(ctx, service.NewSystemTaskProgressReporter(task, runnerID))
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, summary, nil)
}

// asyncTaskPollHandler runs one async-task (Suno/video) polling pass per
// scheduled run. Like midjourneyPollHandler, Enabled() folds in the unfinished
// task existence check so an idle system schedules no rows.
type asyncTaskPollHandler struct{}

func (asyncTaskPollHandler) Type() string { return model.SystemTaskTypeAsyncTaskPoll }

func (asyncTaskPollHandler) Enabled() bool {
	return constant.UpdateTask && model.HasUnfinishedSyncTasks()
}

func (asyncTaskPollHandler) Interval() time.Duration { return 15 * time.Second }

func (asyncTaskPollHandler) NewPayload() any { return nil }

func (asyncTaskPollHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	summary := service.RunTaskPollingOnce(ctx, service.NewSystemTaskProgressReporter(task, runnerID))
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, summary, nil)
}

func finishSystemTaskHandler(task *model.SystemTask, runnerID string, status model.SystemTaskStatus, result any, runErr error) {
	errorMessage := ""
	if runErr != nil {
		errorMessage = runErr.Error()
	}
	if err := model.FinishSystemTask(task.TaskID, runnerID, status, result, errorMessage); err != nil {
		common.SysLog(fmt.Sprintf("system task %s failed to persist result: %v", task.TaskID, err))
	}
}
