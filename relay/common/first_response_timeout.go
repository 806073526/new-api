package common

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/relaykit/types"
)

const FirstResponseTimeoutMessage = "upstream first response timeout"

type firstResponseTimeoutState struct {
	sync.Mutex
	timer      *time.Timer
	cancel     context.CancelFunc
	generation uint64
	timedOut   bool
	received   bool
}

func (state *firstResponseTimeoutState) stopTimerLocked() {
	if state.timer != nil {
		state.timer.Stop()
		state.timer = nil
	}
}

func (info *RelayInfo) StartFirstResponseTimeout(parent context.Context, timeout time.Duration) context.Context {
	if parent == nil {
		parent = context.Background()
	}
	if info == nil {
		return parent
	}

	state := &info.firstResponseTimeout
	state.Lock()
	state.stopTimerLocked()
	if state.cancel != nil {
		state.cancel()
		state.cancel = nil
	}
	state.generation++
	state.timedOut = false
	state.received = false
	if timeout <= 0 {
		state.Unlock()
		return parent
	}

	ctx, cancel := context.WithCancel(parent)
	generation := state.generation
	state.cancel = cancel
	state.timer = time.AfterFunc(timeout, func() {
		state.Lock()
		if state.generation != generation || state.received {
			state.Unlock()
			return
		}
		state.timedOut = true
		cancel := state.cancel
		state.Unlock()
		if cancel != nil {
			cancel()
		}
	})
	state.Unlock()
	return ctx
}

func (info *RelayInfo) FinishFirstResponseTimeout() {
	if info == nil {
		return
	}
	state := &info.firstResponseTimeout
	state.Lock()
	state.generation++
	state.stopTimerLocked()
	cancel := state.cancel
	state.cancel = nil
	state.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (info *RelayInfo) HasFirstResponseTimedOut() bool {
	if info == nil {
		return false
	}
	state := &info.firstResponseTimeout
	state.Lock()
	defer state.Unlock()
	return state.timedOut
}

func NewFirstResponseTimeoutError() *types.NewAPIError {
	return types.NewOpenAIError(
		errors.New(FirstResponseTimeoutMessage),
		types.ErrorCodeFirstResponseTimeout,
		http.StatusGatewayTimeout,
	)
}
