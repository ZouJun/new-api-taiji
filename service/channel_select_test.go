package service

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestCacheGetRandomSatisfiedChannelSwitchesGroupByPerGroupRetryLimit(t *testing.T) {
	setupChannelSelectTestDB(t)
	prepareChannelSelectStrategyOptions(t)
	require.NoError(t, operation_setting.UpdateGroupStrategySettingsByJSONString(`{"default":{"enabled":true,"retry_times":0},"vip":{"enabled":true,"retry_times":2}}`))
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["default","vip"]`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default","vip":"VIP"}`))

	seedSelectableChannel(t, 1, "default", "gpt-test", 10)
	seedSelectableChannel(t, 2, "vip", "gpt-test", 10)

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(nil)
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(ctx, constant.ContextKeyTokenCrossGroupRetry, true)

	param := &RetryParam{
		Ctx:        ctx,
		TokenGroup: "auto",
		ModelName:  "gpt-test",
		Retry:      common.GetPointer(0),
	}

	channel, group, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, channel)
	require.Equal(t, "default", group)
	require.Equal(t, 1, channel.Id)
	require.Equal(t, 1, common.GetContextKeyInt(ctx, constant.ContextKeyAutoGroupIndex))
	require.Equal(t, 0, param.GetRetry())

	param.IncreaseRetry()
	channel, group, err = CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, channel)
	require.Equal(t, "vip", group)
	require.Equal(t, 2, channel.Id)
}

func TestCacheGetRandomSatisfiedChannelFallsThroughWhenGroupHasNoChannel(t *testing.T) {
	setupChannelSelectTestDB(t)
	prepareChannelSelectStrategyOptions(t)
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["default","vip"]`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default","vip":"VIP"}`))

	seedSelectableChannel(t, 2, "vip", "gpt-test", 10)

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(nil)
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(ctx, constant.ContextKeyTokenCrossGroupRetry, true)

	param := &RetryParam{
		Ctx:        ctx,
		TokenGroup: "auto",
		ModelName:  "gpt-test",
		Retry:      common.GetPointer(3),
	}

	channel, group, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, channel)
	require.Equal(t, "vip", group)
	require.Equal(t, 2, channel.Id)
	require.Equal(t, 0, param.GetRetry())
	require.Equal(t, "vip", common.GetContextKeyString(ctx, constant.ContextKeyAutoGroup))
}

func setupChannelSelectTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	prepareChannelSelectColumnNames(t)

	gin.SetMode(gin.TestMode)
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db

	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}))

	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})

	return db
}

func prepareChannelSelectColumnNames(t *testing.T) {
	t.Helper()

	originalIsMasterNode := common.IsMasterNode
	originalSQLitePath := common.SQLitePath
	originalUsingSQLite := common.UsingSQLite
	originalUsingMySQL := common.UsingMySQL
	originalUsingPostgreSQL := common.UsingPostgreSQL
	originalSQLDSN, hadSQLDSN := os.LookupEnv("SQL_DSN")
	defer func() {
		common.IsMasterNode = originalIsMasterNode
		common.SQLitePath = originalSQLitePath
		common.UsingSQLite = originalUsingSQLite
		common.UsingMySQL = originalUsingMySQL
		common.UsingPostgreSQL = originalUsingPostgreSQL
		if hadSQLDSN {
			require.NoError(t, os.Setenv("SQL_DSN", originalSQLDSN))
		} else {
			require.NoError(t, os.Unsetenv("SQL_DSN"))
		}
	}()

	common.IsMasterNode = false
	common.SQLitePath = fmt.Sprintf("file:%s_init?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	common.UsingSQLite = false
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	require.NoError(t, os.Setenv("SQL_DSN", "local"))
	require.NoError(t, model.InitDB())

	if model.DB != nil {
		sqlDB, err := model.DB.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	}
}

func prepareChannelSelectStrategyOptions(t *testing.T) {
	t.Helper()

	common.OptionMapRWMutex.Lock()
	if common.OptionMap == nil {
		common.OptionMap = make(map[string]string)
	}
	common.OptionMap["GroupRatio"] = `{"default":1,"vip":1}`
	common.OptionMapRWMutex.Unlock()

	originalAutoGroups := setting.AutoGroups2JsonString()
	originalUserGroups := setting.UserUsableGroups2JSONString()
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateAutoGroupsByJsonString(originalAutoGroups))
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(originalUserGroups))
		_ = operation_setting.UpdateGroupStrategySettingsByJSONString("{}")
	})
}

func seedSelectableChannel(t *testing.T, id int, group string, modelName string, priority int64) {
	t.Helper()

	weight := uint(0)
	autoBan := 1
	channel := &model.Channel{
		Id:       id,
		Name:     fmt.Sprintf("%s-%d", group, id),
		Key:      "sk-test",
		Status:   common.ChannelStatusEnabled,
		Group:    group,
		Models:   modelName,
		Weight:   &weight,
		Priority: &priority,
		AutoBan:  &autoBan,
	}
	require.NoError(t, model.DB.Create(channel).Error)
	require.NoError(t, channel.AddAbilities(nil))
}
