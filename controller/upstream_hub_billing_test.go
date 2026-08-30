package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestUpstreamHubBillingAggregateExportsSignedQuotaWithM2MAuth(t *testing.T) {
	previousLogDB := model.LOG_DB
	previousMainDatabaseType, previousLogDatabaseType := common.MainDatabaseType(), common.LogDatabaseType()
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		model.LOG_DB = previousLogDB
		common.SetDatabaseTypes(previousMainDatabaseType, previousLogDatabaseType)
	})

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.LOG_DB = db
	t.Cleanup(func() {
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	require.NoError(t, db.AutoMigrate(&model.Log{}, &model.Channel{}))
	require.NoError(t, db.Create(&model.Channel{Id: 12, Name: "渠道十二", Key: "test-key"}).Error)
	require.NoError(t, db.Create(&model.Log{
		CreatedAt: 1704067212,
		Type:      model.LogTypeConsume,
		Quota:     1400000,
		ChannelId: 12,
		Group:     "vip",
		ModelName: "gpt-4o",
		Other:     `{"group_ratio":1.4,"user_group_ratio":0}`,
	}).Error)
	require.NoError(t, db.Create(&model.Log{
		CreatedAt: 1704067220,
		Type:      model.LogTypeRefund,
		Quota:     140000,
		ChannelId: 12,
		Group:     "vip",
		ModelName: "gpt-4o",
		Other:     `{"group_ratio":1.4}`,
	}).Error)

	t.Setenv("UPSTREAM_HUB_API_TOKEN", "billing-test-token")
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/api/internal/upstream-hub")
	group.Use(middleware.UpstreamHubAuth())
	group.POST("/billing/aggregate", GetUpstreamHubBillingAggregate)

	unauthorized := httptest.NewRecorder()
	router.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodPost, "/api/internal/upstream-hub/billing/aggregate", strings.NewReader(`{"start_at":1704067200,"end_at":1704067500,"bucket_seconds":300}`)))
	assert.Equal(t, http.StatusUnauthorized, unauthorized.Code)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/internal/upstream-hub/billing/aggregate", strings.NewReader(`{"start_at":1704067200,"end_at":1704067500,"bucket_seconds":300}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer billing-test-token")
	router.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

	var response struct {
		Success bool `json:"success"`
		Data    struct {
			QuotaPerUnit float64                          `json:"quota_per_unit"`
			Items        []model.UpstreamHubBillingBucket `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.True(t, response.Success)
	assert.Equal(t, common.QuotaPerUnit, response.Data.QuotaPerUnit)
	require.Len(t, response.Data.Items, 1)
	assert.Equal(t, int64(1400000), response.Data.Items[0].ConsumeQuota)
	assert.Equal(t, int64(140000), response.Data.Items[0].RefundQuota)
	assert.Equal(t, int64(1260000), response.Data.Items[0].NetQuota)
	assert.Equal(t, 1.4, response.Data.Items[0].EffectiveGroupRatio)
	assert.Equal(t, "group_ratio", response.Data.Items[0].RatioSource)
	assert.Equal(t, "渠道十二", response.Data.Items[0].ChannelName)
}

func TestUpstreamHubBillingDetailsExportsAuditableLogRowsWithPagination(t *testing.T) {
	previousLogDB := model.LOG_DB
	previousMainDatabaseType, previousLogDatabaseType := common.MainDatabaseType(), common.LogDatabaseType()
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		model.LOG_DB = previousLogDB
		common.SetDatabaseTypes(previousMainDatabaseType, previousLogDatabaseType)
	})

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.LOG_DB = db
	t.Cleanup(func() {
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	for _, item := range []model.Log{
		{Id: 21, CreatedAt: 1704067212, Type: model.LogTypeConsume, Quota: 1400000, UserId: 8, ChannelId: 12, Group: "vip", ModelName: "gpt-4o", TokenName: "token-a", RequestId: "req-a", UpstreamRequestId: "up-a", Other: `{"group_ratio":1.4}`},
		{Id: 22, CreatedAt: 1704067220, Type: model.LogTypeRefund, Quota: 140000, UserId: 8, ChannelId: 12, Group: "vip", ModelName: "gpt-4o", TokenName: "token-a", RequestId: "req-b", Other: `{"group_ratio":1.4}`},
	} {
		require.NoError(t, db.Create(&item).Error)
	}

	t.Setenv("UPSTREAM_HUB_API_TOKEN", "billing-test-token")
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/api/internal/upstream-hub")
	group.Use(middleware.UpstreamHubAuth())
	group.POST("/billing/details", GetUpstreamHubBillingDetails)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/internal/upstream-hub/billing/details", strings.NewReader(`{"start_at":1704067200,"end_at":1704067500,"page":2,"page_size":1}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer billing-test-token")
	router.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

	var response struct {
		Success bool `json:"success"`
		Data    struct {
			Total   int64                           `json:"total"`
			HasMore bool                            `json:"has_more"`
			Items   []model.UpstreamHubBillingEvent `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.True(t, response.Success)
	assert.Equal(t, int64(2), response.Data.Total)
	assert.False(t, response.Data.HasMore)
	require.Len(t, response.Data.Items, 1)
	assert.Equal(t, int64(21), response.Data.Items[0].SourceLogID)
	assert.Equal(t, "consume", response.Data.Items[0].EventType)
	assert.Equal(t, "req-a", response.Data.Items[0].RequestID)
}

func TestUpstreamHubBillingAggregateExportsRootPersonalUsage(t *testing.T) {
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMainDatabaseType, previousLogDatabaseType := common.MainDatabaseType(), common.LogDatabaseType()
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMainDatabaseType, previousLogDatabaseType)
	})
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB, model.LOG_DB = db, db
	t.Cleanup(func() {
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Log{}))
	require.NoError(t, db.Create(&model.User{Id: 101, Username: "root-a", Password: "password", Role: common.RoleRootUser, AffCode: "root-a"}).Error)
	require.NoError(t, db.Create(&model.User{Id: 102, Username: "root-b", Password: "password", Role: common.RoleRootUser, AffCode: "root-b"}).Error)
	require.NoError(t, db.Create(&model.User{Id: 103, Username: "normal", Password: "password", Role: common.RoleCommonUser, AffCode: "normal"}).Error)
	for _, item := range []model.Log{
		{CreatedAt: 1704067212, Type: model.LogTypeConsume, Quota: 100, UserId: 101},
		{CreatedAt: 1704067220, Type: model.LogTypeConsume, Quota: 200, UserId: 102},
		{CreatedAt: 1704067230, Type: model.LogTypeRefund, Quota: 50, UserId: 101},
		{CreatedAt: 1704067240, Type: model.LogTypeConsume, Quota: 999, UserId: 103},
	} {
		require.NoError(t, db.Create(&item).Error)
	}

	t.Setenv("UPSTREAM_HUB_API_TOKEN", "billing-test-token")
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/api/internal/upstream-hub")
	group.Use(middleware.UpstreamHubAuth())
	group.POST("/billing/aggregate", GetUpstreamHubBillingAggregate)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/internal/upstream-hub/billing/aggregate", strings.NewReader(`{"start_at":1704067200,"end_at":1704067500,"bucket_seconds":300}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer billing-test-token")
	router.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			PersonalUsageComplete bool                                   `json:"personal_usage_complete"`
			Items                 []model.UpstreamHubPersonalUsageBucket `json:"personal_usage_items"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.True(t, response.Success)
	assert.True(t, response.Data.PersonalUsageComplete)
	require.Len(t, response.Data.Items, 1)
	assert.Equal(t, int64(300), response.Data.Items[0].ConsumeQuota)
	assert.Equal(t, int64(50), response.Data.Items[0].RefundQuota)
	assert.Equal(t, int64(250), response.Data.Items[0].NetQuota)
}

func TestUpstreamHubSetupExportsInitializedAtWithM2MAuth(t *testing.T) {
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMainDatabaseType, previousLogDatabaseType := common.MainDatabaseType(), common.LogDatabaseType()
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMainDatabaseType, previousLogDatabaseType)
	})
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB, model.LOG_DB = db, db
	t.Cleanup(func() {
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	require.NoError(t, db.AutoMigrate(&model.Setup{}))
	require.NoError(t, db.Create(&model.Setup{Version: "test", InitializedAt: 1704067200}).Error)

	t.Setenv("UPSTREAM_HUB_API_TOKEN", "billing-test-token")
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/api/internal/upstream-hub")
	group.Use(middleware.UpstreamHubAuth())
	group.GET("/setup", GetUpstreamHubSetup)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/internal/upstream-hub/setup", nil)
	request.Header.Set("Authorization", "Bearer billing-test-token")
	router.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

	var response struct {
		Success bool `json:"success"`
		Data    struct {
			InitializedAt int64 `json:"initialized_at"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.True(t, response.Success)
	assert.Equal(t, int64(1704067200), response.Data.InitializedAt)
}

func TestUpstreamHubIdentitiesIncludeDisabledChannels(t *testing.T) {
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMainDatabaseType, previousLogDatabaseType := common.MainDatabaseType(), common.LogDatabaseType()
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMainDatabaseType, previousLogDatabaseType)
	})

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB, model.LOG_DB = db, db
	t.Cleanup(func() {
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	require.NoError(t, db.AutoMigrate(&model.Channel{}))
	require.NoError(t, db.Create(&model.Channel{
		Id: 40, Name: "停用渠道四十", Key: "disabled-key", Status: common.ChannelStatusManuallyDisabled,
	}).Error)

	t.Setenv("UPSTREAM_HUB_API_TOKEN", "billing-test-token")
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/api/internal/upstream-hub")
	group.Use(middleware.UpstreamHubAuth())
	group.GET("/identities", GetUpstreamHubIdentities)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/internal/upstream-hub/identities", nil)
	request.Header.Set("Authorization", "Bearer billing-test-token")
	router.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

	var response struct {
		Success bool                  `json:"success"`
		Data    []upstreamHubIdentity `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.True(t, response.Success)
	require.Len(t, response.Data, 1)
	assert.Equal(t, 40, response.Data[0].ChannelID)
	assert.Equal(t, "停用渠道四十", response.Data[0].ChannelName)
}

func TestUpstreamHubChannelNamesFallsBackToLogDB(t *testing.T) {
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMainDatabaseType, previousLogDatabaseType := common.MainDatabaseType(), common.LogDatabaseType()
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMainDatabaseType, previousLogDatabaseType)
	})

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	logDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = nil
	model.LOG_DB = logDB
	t.Cleanup(func() {
		sqlDB, dbErr := logDB.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	require.NoError(t, logDB.AutoMigrate(&model.Channel{}))
	require.NoError(t, logDB.Create(&model.Channel{Id: 40, Name: "渠道四十", Key: "test-key"}).Error)

	names := upstreamHubChannelNames()
	require.Equal(t, "渠道四十", names[40])
}
