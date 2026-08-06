package controller

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShouldRetryFirstResponseTimeoutWhenGatewayTimeoutIsNormallySkipped(t *testing.T) {
	previous := operation_setting.AutomaticRetryStatusCodeRanges
	operation_setting.AutomaticRetryStatusCodeRanges = []operation_setting.StatusCodeRange{
		{Start: http.StatusInternalServerError, End: http.StatusServiceUnavailable},
	}
	t.Cleanup(func() {
		operation_setting.AutomaticRetryStatusCodeRanges = previous
	})

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	timeoutErr := types.NewErrorWithStatusCode(
		errors.New("upstream first response timeout"),
		types.ErrorCode("first_response_timeout"),
		http.StatusGatewayTimeout,
	)

	assert.True(t, shouldRetry(c, timeoutErr, 1))
}

func TestShouldNotRetryAfterClientDisconnectsBeforeFirstResponseTimeout(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(ctx)
	timeoutErr := types.NewErrorWithStatusCode(
		errors.New("upstream first response timeout"),
		types.ErrorCode("first_response_timeout"),
		http.StatusServiceUnavailable,
	)

	require.Error(t, c.Request.Context().Err())
	assert.False(t, shouldRetry(c, timeoutErr, 1))
}
