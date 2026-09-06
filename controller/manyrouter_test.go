/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

package controller

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	manyrouterservice "github.com/QuantumNous/new-api/service/manyrouter"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func TestFilterManyRouterProductsRequiresOpenUsableGroup(t *testing.T) {
	t.Parallel()
	snapshot := manyrouterservice.Snapshot{Products: []manyrouterservice.Product{
		{Model: "a", GroupKey: "allowed", EntryOpen: true},
		{Model: "b", GroupKey: "closed", EntryOpen: false},
		{Model: "c", GroupKey: "other", EntryOpen: true},
	}}
	filtered := filterManyRouterProducts(snapshot, map[string]string{"allowed": "Allowed", "closed": "Closed"})
	require.Len(t, filtered.Products, 1)
	require.Equal(t, "allowed", filtered.Products[0].GroupKey)
}

func TestManyRouterManagedSyncContract(t *testing.T) {
	router, upstreamURL, upstreamRequests := setupManyRouterSyncTest(t)
	const currentToken = "current-manyrouter-sync-token-1234567890"
	const previousToken = "previous-manyrouter-sync-token-12345678"

	wrongAuth := performManyRouterSyncRequest(t, router, http.MethodGet, "/api/manyrouter/sync/capabilities", "wrong-token-value-that-is-long-enough", nil)
	assert.Equal(t, http.StatusUnauthorized, wrongAuth.Code)
	previousAuth := performManyRouterSyncRequest(t, router, http.MethodGet, "/api/manyrouter/sync/capabilities", previousToken, nil)
	assert.Equal(t, http.StatusOK, previousAuth.Code)
	var capabilities struct {
		Success bool                           `json:"success"`
		Data    dto.ManyRouterSyncCapabilities `json:"data"`
	}
	require.NoError(t, common.Unmarshal(previousAuth.Body.Bytes(), &capabilities))
	assert.True(t, capabilities.Success)
	assert.Equal(t, manyrouterservice.SyncContractVersion, capabilities.Data.ContractVersion)
	assert.Equal(t, "sqlite", capabilities.Data.DatabaseType)
	assert.True(t, capabilities.Data.Features.AtomicApply)

	stateResponse := performManyRouterSyncRequest(t, router, http.MethodGet, "/api/manyrouter/sync/state", currentToken, nil)
	require.Equal(t, http.StatusOK, stateResponse.Code)
	var stateEnvelope struct {
		Success bool                       `json:"success"`
		Data    dto.ManyRouterManagedState `json:"data"`
	}
	require.NoError(t, common.Unmarshal(stateResponse.Body.Bytes(), &stateEnvelope))
	require.True(t, stateEnvelope.Success)
	require.Len(t, stateEnvelope.Data.Channels, 1)
	require.Len(t, stateEnvelope.Data.StateHash, 64)
	assert.Equal(t, "manually_disabled", stateEnvelope.Data.Channels[0].Status)

	request := manyRouterSyncTestRequest(stateEnvelope.Data.StateHash, upstreamURL)
	applyResponse := performManyRouterSyncRequest(t, router, http.MethodPut, "/api/manyrouter/sync/state", currentToken, request)
	require.Equal(t, http.StatusOK, applyResponse.Code, applyResponse.Body.String())
	var applyEnvelope struct {
		Success bool                        `json:"success"`
		Data    dto.ManyRouterApplyResponse `json:"data"`
	}
	require.NoError(t, common.Unmarshal(applyResponse.Body.Bytes(), &applyEnvelope))
	require.True(t, applyEnvelope.Success)
	assert.False(t, applyEnvelope.Data.Replayed)
	require.Len(t, applyEnvelope.Data.State.Channels, 2)
	statuses := map[string]string{}
	for _, channel := range applyEnvelope.Data.State.Channels {
		statuses[channel.ManagedTag] = channel.Status
	}
	assert.Equal(t, "manually_disabled", statuses[request.Channels[0].ManagedTag])
	assert.Equal(t, "enabled", statuses[request.Channels[1].ManagedTag])
	assert.Equal(t, int64(1), upstreamRequests.Load())

	var unmanaged model.Channel
	require.NoError(t, model.DB.Where("name = ?", "operator-owned").First(&unmanaged).Error)
	assert.Equal(t, "operator-secret", unmanaged.Key)
	assert.Equal(t, "custom", unmanaged.Group)
	var options []model.Option
	require.NoError(t, model.DB.Find(&options).Error)
	optionValues := map[string]string{}
	for _, option := range options {
		optionValues[option.Key] = option.Value
	}
	var ratios map[string]float64
	require.NoError(t, common.UnmarshalJsonStr(optionValues["GroupRatio"], &ratios))
	assert.Equal(t, 2.0, ratios["custom"])
	assert.Equal(t, 1.2, ratios["mrab"])
	var usable map[string]string
	require.NoError(t, common.UnmarshalJsonStr(optionValues["UserUsableGroups"], &usable))
	assert.Equal(t, "Operator group", usable["custom"])
	assert.Equal(t, "Balanced", usable["mrab"])
	var receiptCount int64
	require.NoError(t, model.DB.Model(&model.ManyRouterSyncReceipt{}).Count(&receiptCount).Error)
	assert.Equal(t, int64(1), receiptCount)

	replay := performManyRouterSyncRequest(t, router, http.MethodPut, "/api/manyrouter/sync/state", currentToken, request)
	require.Equal(t, http.StatusOK, replay.Code)
	require.NoError(t, common.Unmarshal(replay.Body.Bytes(), &applyEnvelope))
	assert.True(t, applyEnvelope.Data.Replayed)

	changedOperation := request
	changedOperation.RoutePlanVersion++
	operationConflict := performManyRouterSyncRequest(t, router, http.MethodPut, "/api/manyrouter/sync/state", currentToken, changedOperation)
	assert.Equal(t, http.StatusConflict, operationConflict.Code)

	stale := request
	stale.OperationID = "m4-contract-operation-stale"
	stale.ExpectedStateHash = stateEnvelope.Data.StateHash
	staleResponse := performManyRouterSyncRequest(t, router, http.MethodPut, "/api/manyrouter/sync/state", currentToken, stale)
	assert.Equal(t, http.StatusConflict, staleResponse.Code)

	resume := request
	resume.OperationID = "m4-contract-operation-resume"
	resume.RoutePlanVersion++
	resume.ExpectedStateHash = applyEnvelope.Data.State.StateHash
	resume.Channels[0].Resume = true
	resumeResponse := performManyRouterSyncRequest(t, router, http.MethodPut, "/api/manyrouter/sync/state", currentToken, resume)
	require.Equal(t, http.StatusOK, resumeResponse.Code, resumeResponse.Body.String())
	require.NoError(t, common.Unmarshal(resumeResponse.Body.Bytes(), &applyEnvelope))
	statuses = map[string]string{}
	for _, channel := range applyEnvelope.Data.State.Channels {
		statuses[channel.ManagedTag] = channel.Status
	}
	assert.Equal(t, "enabled", statuses[request.Channels[0].ManagedTag])
	assert.Equal(t, int64(2), upstreamRequests.Load())

	unknownField := []byte(`{"contract_version":"m4-managed-sync-v1","unexpected":true}`)
	strictResponse := performManyRouterSyncRequest(t, router, http.MethodPut, "/api/manyrouter/sync/state", currentToken, unknownField)
	assert.Equal(t, http.StatusBadRequest, strictResponse.Code)
}

func TestManyRouterManagedSyncDatabaseMatrix(t *testing.T) {
	tests := []struct {
		name     string
		env      string
		database common.DatabaseType
		open     func(string, *gorm.Config) (*gorm.DB, error)
	}{
		{
			name: "mysql", env: "NEW_API_TEST_MYSQL_DSN", database: common.DatabaseTypeMySQL,
			open: func(dsn string, config *gorm.Config) (*gorm.DB, error) { return gorm.Open(mysql.Open(dsn), config) },
		},
		{
			name: "postgres", env: "NEW_API_TEST_POSTGRES_DSN", database: common.DatabaseTypePostgreSQL,
			open: func(dsn string, config *gorm.Config) (*gorm.DB, error) { return gorm.Open(postgres.Open(dsn), config) },
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			dsn := strings.TrimSpace(os.Getenv(testCase.env))
			if dsn == "" {
				t.Skip(testCase.env + " is not configured")
			}
			prefix := fmt.Sprintf("m4_sync_%d_", time.Now().UnixNano())
			database, err := testCase.open(dsn, &gorm.Config{NamingStrategy: schema.NamingStrategy{TablePrefix: prefix}})
			require.NoError(t, err)
			runManyRouterSyncDatabaseContract(t, database, testCase.database)
		})
	}
}

func runManyRouterSyncDatabaseContract(t *testing.T, database *gorm.DB, databaseType common.DatabaseType) {
	t.Helper()
	previousDB := model.DB
	previousLogDB := model.LOG_DB
	previousMainType := common.MainDatabaseType()
	previousLogType := common.LogDatabaseType()
	previousOptionMap := common.OptionMap
	previousMemoryCache := common.MemoryCacheEnabled
	previousRedis := common.RedisEnabled
	previousRatios := ratio_setting.GroupRatio2JSONString()
	previousUsable := setting.UserUsableGroups2JSONString()
	t.Cleanup(func() {
		model.DB = previousDB
		model.LOG_DB = previousLogDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
		common.OptionMap = previousOptionMap
		common.MemoryCacheEnabled = previousMemoryCache
		common.RedisEnabled = previousRedis
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(previousRatios))
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(previousUsable))
		model.InitChannelCache()
	})
	model.DB = database
	model.LOG_DB = database
	common.SetDatabaseTypes(databaseType, databaseType)
	common.OptionMap = map[string]string{}
	common.MemoryCacheEnabled = false
	common.RedisEnabled = false

	models := []any{&model.Channel{}, &model.Ability{}, &model.Option{}, &model.ManyRouterSyncReceipt{}}
	require.NoError(t, database.AutoMigrate(models...))
	require.NoError(t, database.AutoMigrate(models...))
	t.Cleanup(func() {
		require.NoError(t, database.Migrator().DropTable(
			&model.ManyRouterSyncReceipt{}, &model.Ability{}, &model.Channel{}, &model.Option{},
		))
	})
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"custom":2}`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default","custom":"Operator group"}`))
	require.NoError(t, database.Create(&model.Option{Key: "GroupRatio", Value: ratio_setting.GroupRatio2JSONString()}).Error)
	require.NoError(t, database.Create(&model.Option{Key: "UserUsableGroups", Value: setting.UserUsableGroups2JSONString()}).Error)
	model.InitOptionMap()

	state, err := manyrouterservice.ReadManagedState()
	require.NoError(t, err)
	request := manyRouterSyncTestRequest(state.StateHash, "https://upstream.example")
	normalized, err := manyrouterservice.NormalizeApplyRequest(request)
	require.NoError(t, err)
	payload, err := common.Marshal(normalized)
	require.NoError(t, err)
	digest := sha256.Sum256(payload)
	response, err := model.ApplyManyRouterManagedState(normalized, hex.EncodeToString(digest[:]), time.Now())
	require.NoError(t, err)
	require.Len(t, response.State.Channels, 2)
	require.Len(t, response.State.Groups, 3)
	replay, err := model.ApplyManyRouterManagedState(normalized, hex.EncodeToString(digest[:]), time.Now())
	require.NoError(t, err)
	assert.True(t, replay.Replayed)
	var receipts int64
	require.NoError(t, database.Model(&model.ManyRouterSyncReceipt{}).Count(&receipts).Error)
	assert.Equal(t, int64(1), receipts)
}

func setupManyRouterSyncTest(t *testing.T) (*gin.Engine, string, *atomic.Int64) {
	t.Helper()
	previousDB := model.DB
	previousLogDB := model.LOG_DB
	previousMainType := common.MainDatabaseType()
	previousLogType := common.LogDatabaseType()
	previousOptionMap := common.OptionMap
	previousMemoryCache := common.MemoryCacheEnabled
	previousRedis := common.RedisEnabled
	previousSelfUse := operation_setting.SelfUseModeEnabled
	previousRatios := ratio_setting.GroupRatio2JSONString()
	previousUsable := setting.UserUsableGroups2JSONString()

	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, database.AutoMigrate(
		&model.Channel{}, &model.Ability{}, &model.Option{}, &model.Log{}, &model.User{}, &model.ManyRouterSyncReceipt{},
	))
	model.DB = database
	model.LOG_DB = database
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.OptionMap = map[string]string{}
	common.MemoryCacheEnabled = false
	common.RedisEnabled = false
	operation_setting.SelfUseModeEnabled = true
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"custom":2,"mr_s_11111111111111111111111111111111":1.1}`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default","custom":"Operator group","mr_s_11111111111111111111111111111111":"Old managed"}`))
	require.NoError(t, database.Create(&model.Option{Key: "GroupRatio", Value: ratio_setting.GroupRatio2JSONString()}).Error)
	require.NoError(t, database.Create(&model.Option{Key: "UserUsableGroups", Value: setting.UserUsableGroups2JSONString()}).Error)
	model.InitOptionMap()
	require.NoError(t, database.Create(&model.User{
		Id: 1, Username: "root", Password: "test-password", Role: common.RoleRootUser,
		Status: common.UserStatusEnabled, Quota: 1_000_000_000, Group: "default",
	}).Error)
	upstreamRequests := &atomic.Int64{}
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		upstreamRequests.Add(1)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"id":"chatcmpl-test","object":"chat.completion","created":1,"model":"gpt-3.5-turbo","choices":[{"index":0,"message":{"role":"assistant","content":"OK"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	t.Cleanup(upstream.Close)

	priority := int64(0)
	weight := uint(100)
	managedBaseURL := upstream.URL
	managedTag := "manyrouter:11111111-1111-1111-1111-111111111111"
	managed := model.Channel{
		Type: 1, Key: "old-managed-secret", Status: common.ChannelStatusManuallyDisabled, Name: "managed-one",
		Weight: &weight, BaseURL: &managedBaseURL, Models: "gpt-3.5-turbo", Group: "mr_s_11111111111111111111111111111111",
		Priority: &priority, Tag: &managedTag, ChannelInfo: model.ChannelInfo{ManyRouterCredentialVersion: 1},
	}
	require.NoError(t, database.Create(&managed).Error)
	require.NoError(t, managed.AddAbilities(nil))
	unmanagedBaseURL := "https://operator.example/v1"
	unmanaged := model.Channel{
		Type: 1, Key: "operator-secret", Status: common.ChannelStatusEnabled, Name: "operator-owned",
		Weight: &weight, BaseURL: &unmanagedBaseURL, Models: "gpt-3.5-turbo", Group: "custom", Priority: &priority,
	}
	require.NoError(t, database.Create(&unmanaged).Error)
	require.NoError(t, unmanaged.AddAbilities(nil))
	model.InitChannelCache()

	t.Setenv("MANYROUTER_SYNC_TOKEN", "current-manyrouter-sync-token-1234567890")
	t.Setenv("MANYROUTER_SYNC_TOKEN_PREVIOUS", "previous-manyrouter-sync-token-12345678")
	t.Cleanup(func() {
		model.DB = previousDB
		model.LOG_DB = previousLogDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
		common.OptionMap = previousOptionMap
		common.MemoryCacheEnabled = previousMemoryCache
		common.RedisEnabled = previousRedis
		operation_setting.SelfUseModeEnabled = previousSelfUse
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(previousRatios))
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(previousUsable))
		model.InitChannelCache()
	})

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	group := engine.Group("/api/manyrouter/sync")
	group.Use(middleware.ManyRouterSyncAuth())
	group.GET("/capabilities", GetManyRouterSyncCapabilities)
	group.GET("/state", GetManyRouterManagedState)
	group.PUT("/state", ApplyManyRouterManagedState)
	return engine, upstream.URL, upstreamRequests
}

func manyRouterSyncTestRequest(expectedHash, upstreamURL string) dto.ManyRouterApplyRequest {
	return dto.ManyRouterApplyRequest{
		ContractVersion:   manyrouterservice.SyncContractVersion,
		OperationID:       "m4-contract-operation-0001",
		RoutePlanVersion:  4,
		ExpectedStateHash: expectedHash,
		Channels: []dto.ManyRouterDesiredChannel{
			{
				ManagedTag: "manyrouter:11111111-1111-1111-1111-111111111111", Name: "managed-one",
				BaseURL: upstreamURL, APIKey: "managed-secret-one", CredentialVersion: 2,
				Models: []dto.ManyRouterSyncModel{{Model: "gpt-3.5-turbo", UpstreamModel: "gpt-3.5-turbo"}},
				Groups: []string{"mr_s_11111111111111111111111111111111", "mrab"}, Weight: 100,
				DesiredStatus: "enabled",
			},
			{
				ManagedTag: "manyrouter:22222222-2222-2222-2222-222222222222", Name: "managed-two",
				BaseURL: upstreamURL, APIKey: "managed-secret-two", CredentialVersion: 1,
				Models: []dto.ManyRouterSyncModel{{Model: "gpt-3.5-turbo", UpstreamModel: "gpt-3.5-turbo"}},
				Groups: []string{"mr_s_22222222222222222222222222222222", "mrab"}, Weight: 100,
				DesiredStatus: "enabled",
			},
		},
		Groups: []dto.ManyRouterManagedGroup{
			{Key: "mr_s_11111111111111111111111111111111", DisplayName: "Managed one", SaleRatio: "1.1", Visible: true},
			{Key: "mr_s_22222222222222222222222222222222", DisplayName: "Managed two", SaleRatio: "1.15", Visible: false},
			{Key: "mrab", DisplayName: "Balanced", SaleRatio: "1.2", Visible: true},
		},
	}
}

func performManyRouterSyncRequest(
	t *testing.T,
	router http.Handler,
	method string,
	path string,
	token string,
	body any,
) *httptest.ResponseRecorder {
	t.Helper()
	var payload []byte
	var err error
	switch value := body.(type) {
	case nil:
	case []byte:
		payload = value
	default:
		payload, err = common.Marshal(value)
		require.NoError(t, err)
	}
	request := httptest.NewRequest(method, path, bytes.NewReader(payload))
	request.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}
