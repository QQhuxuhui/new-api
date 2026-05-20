package setting

import (
	"math"
	"sync/atomic"
)

// USDT (TRC20) payment via self-hosted assimon/epusdt gateway.
//
// 计价模型: 系统原有按 CNY 计费 (Price * amount), USDT 走 CNY → USDT 换算:
//     usdtAmount = cnyAmount / EpUsdtCnyRate
// EpUsdtCnyRate 表示 "1 USDT 折合多少 CNY", 不是真实美元汇率。
// 若运营是纯美元站, 把 EpUsdtCnyRate 设成 1.0 即可兼容。

var (
	// ePUSDT 网关基础配置
	EpUsdtApiUrl   = ""    // 网关 base URL, 例如 https://usdt.example.com
	EpUsdtApiToken = ""    // 网关 API token, 签名密钥
	EpUsdtMinTopUp = 1     // USDT 充值最小额度 (USD 面值)
	EpUsdtTestMode = false // 测试模式: 跳过签名校验, 仅开发环境使用
)

// 汇率与自动刷新配置
// EpUsdtCnyRate 使用 atomic 读写, 因为后台 goroutine 与下单请求并发访问。
var (
	epUsdtCnyRateBits uint64 // float64 通过 math.Float64bits 编码

	EpUsdtRateAuto      = false       // 是否启用自动汇率拉取
	EpUsdtRateSource    = "binance"   // 主源: binance / coingecko
	EpUsdtRateInterval  = 10          // 拉取间隔 (分钟)
	EpUsdtRateMargin    = 0.005       // 加价幅度: final = raw * (1 + margin)
	EpUsdtRateMin       = 5.0         // 拉到的汇率下限, 异常护栏
	EpUsdtRateMax       = 10.0        // 拉到的汇率上限, 异常护栏
	EpUsdtRateStaleSec  = int64(3600) // 超过此秒数未更新视为陈旧
	EpUsdtRateUpdatedAt int64         // 最近一次成功更新时间 (Unix 秒)
)

func init() {
	SetEpUsdtCnyRate(7.2)
}

// GetEpUsdtCnyRate 原子读取当前汇率。
func GetEpUsdtCnyRate() float64 {
	return math.Float64frombits(atomic.LoadUint64(&epUsdtCnyRateBits))
}

// SetEpUsdtCnyRate 原子写入汇率。
func SetEpUsdtCnyRate(v float64) {
	atomic.StoreUint64(&epUsdtCnyRateBits, math.Float64bits(v))
}
