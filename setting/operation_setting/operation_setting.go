package operation_setting

import "strings"

var DemoSiteEnabled = false
var SelfUseModeEnabled = false

// RechargeDisabled 充值总开关：为 true 时关闭全站所有充值/加额度入口（在线支付、USDT 钱包、按量付费下单、套餐购买下单），
// 钱包管理页呈现与「未配置充值地址」一致的空状态。仅拦截「发起新充值」，不影响支付回调入账、管理员补单与兑换码。
var RechargeDisabled = false

var AutomaticDisableKeywords = []string{
	"Your credit balance is too low",
	"This organization has been disabled.",
	"You exceeded your current quota",
	"Permission denied",
	"The security token included in the request is invalid",
	"Operation not allowed",
	"Your account is not authorized",
}

func AutomaticDisableKeywordsToString() string {
	return strings.Join(AutomaticDisableKeywords, "\n")
}

func AutomaticDisableKeywordsFromString(s string) {
	AutomaticDisableKeywords = []string{}
	ak := strings.Split(s, "\n")
	for _, k := range ak {
		k = strings.TrimSpace(k)
		k = strings.ToLower(k)
		if k != "" {
			AutomaticDisableKeywords = append(AutomaticDisableKeywords, k)
		}
	}
}
