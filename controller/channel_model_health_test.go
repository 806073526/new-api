package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
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
