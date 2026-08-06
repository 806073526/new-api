package common

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type firstResponseTimeoutController interface {
	StartFirstResponseTimeout(context.Context, time.Duration) context.Context
	FinishFirstResponseTimeout()
	HasFirstResponseTimedOut() bool
}

func TestFirstResponseTimeoutCancelsAttemptWithoutFirstEvent(t *testing.T) {
	info := &RelayInfo{}
	controller, ok := any(info).(firstResponseTimeoutController)
	require.True(t, ok, "RelayInfo must manage the first-response deadline for a stream attempt")
	defer controller.FinishFirstResponseTimeout()

	ctx := controller.StartFirstResponseTimeout(context.Background(), 20*time.Millisecond)
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("first-response deadline did not cancel the upstream attempt")
	}

	assert.True(t, controller.HasFirstResponseTimedOut())
}

func TestFirstResponseTimeoutStopsAfterFirstEvent(t *testing.T) {
	info := &RelayInfo{}
	controller, ok := any(info).(firstResponseTimeoutController)
	require.True(t, ok, "RelayInfo must manage the first-response deadline for a stream attempt")
	defer controller.FinishFirstResponseTimeout()

	ctx := controller.StartFirstResponseTimeout(context.Background(), 20*time.Millisecond)
	info.SetFirstResponseTime()

	select {
	case <-ctx.Done():
		t.Fatal("first response must keep the upstream stream alive")
	case <-time.After(60 * time.Millisecond):
	}

	assert.False(t, controller.HasFirstResponseTimedOut())
}
