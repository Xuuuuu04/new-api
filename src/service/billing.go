package service

import (
	"fmt"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

const (
	BillingSourceWallet = "wallet"
)

// PreConsumeBilling pre-consumes from wallet quota only (payments/subscriptions disabled).
func PreConsumeBilling(c *gin.Context, preConsumedQuota int, relayInfo *relaycommon.RelayInfo) *types.NewAPIError {
	if relayInfo == nil {
		return types.NewError(fmt.Errorf("relayInfo is nil"), types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}
	relayInfo.BillingSource = BillingSourceWallet
	relayInfo.SubscriptionId = 0
	relayInfo.SubscriptionPreConsumed = 0
	relayInfo.SubscriptionPostDelta = 0
	return PreConsumeQuota(c, preConsumedQuota, relayInfo)
}
