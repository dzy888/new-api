package router

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestCanvasCatalogUsesAuthorizedGroupPriorityAndEffectiveRatio(t *testing.T) {
	oldRatios := ratio_setting.GroupRatio2JSONString()
	oldSpecial := ratio_setting.GroupGroupRatio2JSONString()
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":2,"vip":3,"secret":0.1}`))
	require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(`{"default":{"vip":1.25}}`))
	t.Cleanup(func() {
		_ = ratio_setting.UpdateGroupRatioByJSONString(oldRatios)
		_ = ratio_setting.UpdateGroupGroupRatioByJSONString(oldSpecial)
	})
	prices := []model.Pricing{
		{ModelName: "image-b", EnableGroup: []string{"vip", "default"}, SupportedEndpointTypes: []constant.EndpointType{constant.EndpointTypeImageGeneration}},
		{ModelName: "video-a", EnableGroup: []string{"vip"}, SupportedEndpointTypes: []constant.EndpointType{constant.EndpointTypeOpenAIVideo}},
		{ModelName: "private", EnableGroup: []string{"secret"}, SupportedEndpointTypes: []constant.EndpointType{constant.EndpointTypeImageGeneration}},
		{ModelName: "embedding", EnableGroup: []string{"default"}, SupportedEndpointTypes: []constant.EndpointType{constant.EndpointTypeEmbeddings}},
	}
	offers := service.BuildCanvasModels("default", []string{"default", "vip", "secret"}, map[string]string{"default": "Standard", "vip": "Premium"}, prices)
	require.Len(t, offers, 2)
	assert.Equal(t, "image-b", offers[0].Name)
	assert.Equal(t, "default", offers[0].Group)
	assert.Equal(t, 2.0, offers[0].GroupRatio)
	assert.Equal(t, "vip", offers[1].Group)
	assert.Equal(t, 1.25, offers[1].GroupRatio)
	assert.Equal(t, []string{"video"}, offers[1].Capabilities)
}

func TestCanvasSessionRelayOwnershipAndUserFunding(t *testing.T) {
	oldDB, oldLogs := model.DB, model.LOG_DB
	oldLogType := common.LogDatabaseType()
	oldType, oldRedis, oldBatch, oldSecret := common.MainDatabaseType(), common.RedisEnabled, common.BatchUpdateEnabled, common.SessionSecret
	oldGroups := setting.UserUsableGroups2JSONString()
	oldPrices := ratio_setting.ModelPrice2JSONString()
	oldMemory := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"gpt-image-canvas":0.001}`))
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.UserSession{}, &model.Task{}, &model.Token{}, &model.Channel{}, &model.Ability{}, &model.Model{}, &model.Vendor{}, &model.Log{}, &model.Option{}, &model.AuditLog{}, &model.UserAccessToken{}))
	model.DB, model.LOG_DB = db, db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	t.Setenv("LOG_SQL_DSN", "")
	require.NoError(t, model.InitLogDB())
	require.NoError(t, i18n.Init())
	common.RedisEnabled, common.BatchUpdateEnabled, common.SessionSecret = false, false, "canvas-session-test-secret"
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Standard"}`))
	require.NoError(t, model.EnsureLegacyAccessTokenRetireAt(time.Now().Unix()))
	model.InvalidatePricingCache()
	t.Cleanup(func() {
		model.DB, model.LOG_DB = oldDB, oldLogs
		common.SetMainDatabaseType(oldType)
		common.SetLogDatabaseType(oldLogType)
		common.RedisEnabled, common.BatchUpdateEnabled, common.SessionSecret = oldRedis, oldBatch, oldSecret
		_ = setting.UpdateUserUsableGroupsByJSONString(oldGroups)
		_ = ratio_setting.UpdateModelPriceByJSONString(oldPrices)
		common.MemoryCacheEnabled = oldMemory
		model.InvalidatePricingCache()
		_ = sqlDB.Close()
	})
	user := &model.User{Username: "canvas-owner", Password: "unused", Role: common.RoleAdminUser, Status: common.UserStatusEnabled, Group: "default", Quota: 10000, AuthVersion: 1, Setting: `{"billing_preference":"wallet_only"}`}
	require.NoError(t, db.Create(user).Error)
	session, err := service.CreateLoginSession(user.Id, "password", "127.0.0.1", "canvas-test")
	require.NoError(t, err)
	var upstreamCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls.Add(1)
		assert.Equal(t, "/v1/images/generations", r.URL.Path)
		assert.Equal(t, "Bearer private-upstream-secret", r.Header.Get("Authorization"))
		assert.Empty(t, r.Header.Get("Cookie"))
		assert.Empty(t, r.Header.Get("X-Auth-Session"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"created":1,"data":[{"b64_json":"aW1hZ2U="}]}`))
	}))
	defer upstream.Close()
	channel := &model.Channel{BaseURL: &upstream.URL, Type: constant.ChannelTypeOpenAI, Key: "private-upstream-secret", Status: common.ChannelStatusEnabled, Name: "test", Models: "gpt-image-canvas", Group: "default"}
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, db.Create(&model.Ability{Group: "default", Model: "gpt-image-canvas", ChannelId: channel.Id, Enabled: true}).Error)
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetCanvasRouter(engine)
	engine.GET("/api/pricing", middleware.UserAuth(), controller.GetPricing)
	request := func(method, path, body, token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rec := httptest.NewRecorder()
		engine.ServeHTTP(rec, req)
		return rec
	}
	for _, path := range []string{"/api/pricing", "/api/canvas/relay/v1/videos/other-task"} {
		rec := request(http.MethodGet, path, "", "")
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	}
	assert.Equal(t, http.StatusNotFound, request(http.MethodGet, "/api/canvas/models", "", session.AccessToken).Code)
	catalog := request(http.MethodGet, "/api/pricing", "", session.AccessToken)
	require.Equal(t, http.StatusOK, catalog.Code, catalog.Body.String())
	assert.Contains(t, catalog.Body.String(), "gpt-image-canvas")
	assert.NotContains(t, catalog.Body.String(), channel.Key)
	assert.NotContains(t, catalog.Body.String(), session.AccessToken)
	denied := request(http.MethodPost, "/api/canvas/relay/v1/images/generations", `{"model":"not-authorized","group":"default"}`, session.AccessToken)
	assert.Equal(t, http.StatusForbidden, denied.Code)
	// A terminal probe exercises the production authentication and funding preparation before an upstream call.
	probe := engine.Group("/api/canvas/relay", middleware.UserAuth(), middleware.CanvasSessionOnly(), middleware.CanvasRelayAuth())
	probe.POST("/v1/probe", func(c *gin.Context) {
		assert.Equal(t, "default", common.GetContextKeyString(c, constant.ContextKeyUsingGroup))
		assert.Empty(t, c.Request.Header.Get("Authorization"))
		assert.Empty(t, c.Request.Header.Get("Cookie"))
		info, err := relaycommon.GenRelayInfo(c, types.RelayFormatOpenAI, nil, nil)
		require.NoError(t, err)
		require.True(t, info.IsPlayground)
		assert.Zero(t, info.TokenId)
		info.ForcePreConsume = true
		require.Nil(t, service.PreConsumeBilling(c, 100, info))
		require.NoError(t, service.SettleBilling(c, info, 60))
		c.Status(http.StatusNoContent)
	})
	funded := request(http.MethodPost, "/api/canvas/relay/v1/probe", `{"model":"gpt-image-canvas","group":"secret"}`, session.AccessToken)
	require.Equal(t, http.StatusNoContent, funded.Code, funded.Body.String())
	var balance model.User
	require.NoError(t, db.First(&balance, user.Id).Error)
	assert.Equal(t, 9940, balance.Quota, "only the current wallet pays actual usage, without an API token")
	var tokens int64
	require.NoError(t, db.Model(&model.Token{}).Count(&tokens).Error)
	assert.Zero(t, tokens)
	own := &model.Task{TaskID: "owned-task", UserId: user.Id, Status: model.TaskStatusQueued}
	other := &model.Task{TaskID: "other-task", UserId: user.Id + 1, Status: model.TaskStatusQueued}
	require.NoError(t, db.Create(own).Error)
	require.NoError(t, db.Create(other).Error)
	assert.Equal(t, http.StatusOK, request(http.MethodGet, "/api/canvas/relay/v1/tasks/owned-task", "", session.AccessToken).Code)
	assert.Equal(t, http.StatusNotFound, request(http.MethodGet, "/api/canvas/relay/v1/tasks/other-task", "", session.AccessToken).Code)
	assert.Equal(t, http.StatusNotFound, request(http.MethodGet, "/api/canvas/relay/v1/videos/other-task/content", "", session.AccessToken).Code, "admin canvas access must not read another account's artifacts")
	// Async failures without API-token IDs still return reserved funds to the task owner.
	refund := &model.Task{TaskID: "refund-task", UserId: user.Id, Quota: 25, Status: model.TaskStatusFailure}
	require.NoError(t, db.Create(refund).Error)
	assert.True(t, service.RefundTaskQuota(context.Background(), refund, "test failure"))
	require.NoError(t, db.First(&balance, user.Id).Error)
	assert.Equal(t, 9965, balance.Quota)
	// The real registered image relay validates quantities and swaps session credentials for the channel key.
	invalid := request(http.MethodPost, "/api/canvas/relay/v1/images/generations", `{"model":"gpt-image-canvas","prompt":"test","n":999999999}`, session.AccessToken)
	require.Equal(t, http.StatusBadRequest, invalid.Code, invalid.Body.String())
	assert.Zero(t, upstreamCalls.Load())
	image := request(http.MethodPost, "/api/canvas/relay/v1/images/generations", `{"model":"gpt-image-canvas","prompt":"test","n":1,"size":"1024x1024"}`, session.AccessToken)
	require.Equal(t, http.StatusOK, image.Code, image.Body.String())
	assert.Contains(t, image.Body.String(), "aW1hZ2U=")
	assert.EqualValues(t, 1, upstreamCalls.Load())
	require.NoError(t, db.First(&balance, user.Id).Error)
	assert.Less(t, balance.Quota, 9965)
	require.NoError(t, db.Model(&model.Token{}).Count(&tokens).Error)
	assert.Zero(t, tokens)
	require.NoError(t, db.Model(&model.UserSession{}).Where("sid = ?", session.Session.SID).Update("status", model.UserSessionStatusRevoked).Error)
	revoked := request(http.MethodGet, "/api/pricing", "", session.AccessToken)
	assert.Equal(t, http.StatusUnauthorized, revoked.Code, fmt.Sprint(revoked.Body))
}
