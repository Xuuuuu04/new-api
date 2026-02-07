package helper

import (
	"fmt"
	"math"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

// HandleGroupRatio checks for "auto_group" in the context and updates the group ratio and relayInfo.UsingGroup if present
func HandleGroupRatio(ctx *gin.Context, relayInfo *relaycommon.RelayInfo) types.GroupRatioInfo {
	groupRatioInfo := types.GroupRatioInfo{
		GroupRatio:        1.0, // default ratio
		GroupSpecialRatio: -1,
	}

	// check auto group
	autoGroup, exists := ctx.Get("auto_group")
	if exists {
		logger.LogDebug(ctx, fmt.Sprintf("final group: %s", autoGroup))
		relayInfo.UsingGroup = autoGroup.(string)
	}

	return groupRatioInfo
}

func ModelPriceHelper(c *gin.Context, info *relaycommon.RelayInfo, promptTokens int, meta *types.TokenCountMeta) (types.PriceData, error) {
	groupRatioInfo := HandleGroupRatio(c, info)

	modelPricePer1M, ok := model.GetModelPricePer1M(info.OriginModelName)
	if !ok {
		if !info.UserSetting.AcceptUnsetRatioModel {
			return types.PriceData{}, fmt.Errorf("模型 %s 价格未配置，请联系管理员设置", info.OriginModelName)
		}
		modelPricePer1M = 0
	}

	preConsumedTokens := common.Max(promptTokens, common.PreConsumedQuota)
	if meta.MaxTokens != 0 {
		preConsumedTokens += meta.MaxTokens
	}
	preConsumedQuota := calcQuotaFromTokens(preConsumedTokens, modelPricePer1M)
	freeModel := modelPricePer1M == 0

	// check if free model pre-consume is disabled
	if !operation_setting.GetQuotaSetting().EnableFreeModelPreConsume {
		// if model price or ratio is 0, do not pre-consume quota
		if modelPricePer1M == 0 {
			preConsumedQuota = 0
			freeModel = true
		}
	}

	priceData := types.PriceData{
		FreeModel:            freeModel,
		ModelPrice:           modelPricePer1M,
		ModelRatio:           1,
		CompletionRatio:      1,
		GroupRatioInfo:       groupRatioInfo,
		UsePrice:             true,
		CacheRatio:           1,
		ImageRatio:           1,
		AudioRatio:           1,
		AudioCompletionRatio: 1,
		CacheCreationRatio:   1,
		CacheCreation5mRatio: 1,
		CacheCreation1hRatio: 1,
		QuotaToPreConsume:    preConsumedQuota,
	}

	if common.DebugEnabled {
		println(fmt.Sprintf("model_price_helper result: %s", priceData.ToSetting()))
	}
	info.PriceData = priceData
	return priceData, nil
}

// ModelPriceHelperPerCall 按次计费的 PriceHelper (MJ、Task)
func ModelPriceHelperPerCall(c *gin.Context, info *relaycommon.RelayInfo) types.PerCallPriceData {
	groupRatioInfo := HandleGroupRatio(c, info)
	modelPrice := 0.0
	quota := 0
	priceData := types.PerCallPriceData{
		ModelPrice:     modelPrice,
		Quota:          quota,
		GroupRatioInfo: groupRatioInfo,
	}
	return priceData
}

func ContainPriceOrRatio(modelName string) bool {
	_, ok := model.GetModelPricePer1M(modelName)
	return ok
}

func calcQuotaFromTokens(tokens int, pricePer1M float64) int {
	if tokens <= 0 || pricePer1M <= 0 {
		return 0
	}
	return int(math.Round((float64(tokens) * pricePer1M / 1_000_000.0) * common.QuotaPerUnit))
}
