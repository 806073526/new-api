package service

import (
	"errors"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShouldDisableChannelSkipsFirstResponseTimeout(t *testing.T) {
	previousEnabled := common.AutomaticDisableChannelEnabled
	previousRanges := operation_setting.AutomaticDisableStatusCodeRanges
	common.AutomaticDisableChannelEnabled = true
	operation_setting.AutomaticDisableStatusCodeRanges = []operation_setting.StatusCodeRange{
		{Start: http.StatusGatewayTimeout, End: http.StatusGatewayTimeout},
	}
	t.Cleanup(func() {
		common.AutomaticDisableChannelEnabled = previousEnabled
		operation_setting.AutomaticDisableStatusCodeRanges = previousRanges
	})

	timeoutErr := types.NewErrorWithStatusCode(
		errors.New("upstream first response timeout"),
		types.ErrorCodeFirstResponseTimeout,
		http.StatusGatewayTimeout,
	)

	require.True(t, common.AutomaticDisableChannelEnabled)
	assert.False(t, ShouldDisableChannel(timeoutErr))
}
