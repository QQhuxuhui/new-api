package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
)

// usdtRateHTTPClient 5 秒超时, 避免长时间阻塞 refresher goroutine
var usdtRateHTTPClient = &http.Client{Timeout: 5 * time.Second}

// 防止并发重入刷新 (例如手动触发 + 定时同时跑)
var usdtRateRefreshMu sync.Mutex

// StartEpUsdtRateRefresher 在 main 启动时调用, 常驻运行。
// 每个 tick 内部判断 EpUsdtRateAuto, 关 → 跳过, 开 → 拉取。
// 这样运行时通过后台开启自动模式可即时生效, 无需重启服务。
func StartEpUsdtRateRefresher() {
	interval := setting.EpUsdtRateInterval
	if interval < 5 {
		interval = 5
	}
	go func() {
		// 启动后稍延迟首次拉取, 让 DB 与 OptionMap 初始化完成
		time.Sleep(10 * time.Second)
		if setting.EpUsdtRateAuto {
			RefreshEpUsdtRateOnce()
		}
		ticker := time.NewTicker(time.Duration(interval) * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			if !setting.EpUsdtRateAuto {
				continue // 手动模式静默跳过, 避免每 N 分钟刷一条日志
			}
			RefreshEpUsdtRateOnce()
		}
	}()
	common.SysLog(fmt.Sprintf("usdt rate refresher daemon started: source=%s interval=%dm (auto mode toggled at runtime)",
		setting.EpUsdtRateSource, interval))
}

// RefreshEpUsdtRateOnce 拉取一次, 应用 margin 与边界护栏, 写入设置并持久化。
// 失败时保留旧值。
func RefreshEpUsdtRateOnce() {
	usdtRateRefreshMu.Lock()
	defer usdtRateRefreshMu.Unlock()

	raw, src, err := fetchEpUsdtRate()
	if err != nil {
		common.SysLog(fmt.Sprintf("usdt rate fetch failed: %v (keep old=%.4f)", err, setting.GetEpUsdtCnyRate()))
		return
	}
	if math.IsNaN(raw) || math.IsInf(raw, 0) || raw <= 0 {
		common.SysLog(fmt.Sprintf("usdt rate invalid: %v from %s (rejected)", raw, src))
		return
	}
	if raw < setting.EpUsdtRateMin || raw > setting.EpUsdtRateMax {
		common.SysLog(fmt.Sprintf("usdt rate %.4f from %s out of bounds [%.2f,%.2f] (rejected)",
			raw, src, setting.EpUsdtRateMin, setting.EpUsdtRateMax))
		return
	}
	final := raw * (1 + setting.EpUsdtRateMargin)
	setting.SetEpUsdtCnyRate(final)
	setting.EpUsdtRateUpdatedAt = time.Now().Unix()

	// 持久化 (失败仅记日志, 不影响内存生效)
	if err := model.UpdateOption("EpUsdtCnyRate", strconv.FormatFloat(final, 'f', -1, 64)); err != nil {
		common.SysLog("persist EpUsdtCnyRate failed: " + err.Error())
	}
	if err := model.UpdateOption("EpUsdtRateUpdatedAt", strconv.FormatInt(setting.EpUsdtRateUpdatedAt, 10)); err != nil {
		common.SysLog("persist EpUsdtRateUpdatedAt failed: " + err.Error())
	}
	common.SysLog(fmt.Sprintf("usdt rate updated: %.4f (raw=%.4f +%.2f%% from %s)",
		final, raw, setting.EpUsdtRateMargin*100, src))
}

// fetchEpUsdtRate 按主源拉, 失败降级到 CoinGecko。
// 返回 (raw_rate, source_used, err)。
func fetchEpUsdtRate() (float64, string, error) {
	primary := setting.EpUsdtRateSource
	if primary == "" {
		primary = "binance"
	}
	if r, err := fetchOneSource(primary); err == nil {
		return r, primary, nil
	} else {
		common.SysLog(fmt.Sprintf("usdt rate primary source %s failed: %v, try fallback", primary, err))
	}
	// 兜底: 切到另一个
	fallback := "coingecko"
	if primary == "coingecko" {
		fallback = "binance"
	}
	r, err := fetchOneSource(fallback)
	if err != nil {
		return 0, "", fmt.Errorf("all sources failed (last=%s): %w", fallback, err)
	}
	return r, fallback, nil
}

func fetchOneSource(source string) (float64, error) {
	switch source {
	case "binance":
		return fetchBinanceUsdtCny()
	case "coingecko":
		return fetchCoinGeckoUsdtCny()
	default:
		return 0, fmt.Errorf("unknown source %q", source)
	}
}

// fetchBinanceUsdtCny 抓 Binance C2C USDT/CNY BUY (买家用 CNY 买 USDT) 前 10 单 adv.price 均值。
// adv.price 是商家挂的"1 USDT 卖多少 CNY", 正好就是我们要的 EpUsdtCnyRate。
func fetchBinanceUsdtCny() (float64, error) {
	body := map[string]any{
		"asset":     "USDT",
		"fiat":      "CNY",
		"tradeType": "BUY",
		"rows":      10,
		"page":      1,
	}
	buf, _ := json.Marshal(body)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "POST",
		"https://p2p.binance.com/bapi/c2c/v2/friendly/c2c/adv/search",
		bytes.NewReader(buf))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 NewApi-USDT-Rate")
	resp, err := usdtRateHTTPClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return 0, fmt.Errorf("binance http %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}
	var parsed struct {
		Code string `json:"code"`
		Data []struct {
			Adv struct {
				Price string `json:"price"`
			} `json:"adv"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return 0, fmt.Errorf("binance decode: %w", err)
	}
	if parsed.Code != "000000" && parsed.Code != "" {
		return 0, fmt.Errorf("binance code=%s", parsed.Code)
	}
	if len(parsed.Data) == 0 {
		return 0, errors.New("binance empty data")
	}
	var sum float64
	var n int
	for _, ad := range parsed.Data {
		p, err := strconv.ParseFloat(ad.Adv.Price, 64)
		if err != nil || p <= 0 {
			continue
		}
		sum += p
		n++
	}
	if n == 0 {
		return 0, errors.New("binance no valid prices")
	}
	return sum / float64(n), nil
}

// fetchCoinGeckoUsdtCny 直接读 .tether.cny。
func fetchCoinGeckoUsdtCny() (float64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET",
		"https://api.coingecko.com/api/v3/simple/price?ids=tether&vs_currencies=cny", nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := usdtRateHTTPClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return 0, fmt.Errorf("coingecko http %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}
	var parsed struct {
		Tether struct {
			Cny float64 `json:"cny"`
		} `json:"tether"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return 0, fmt.Errorf("coingecko decode: %w", err)
	}
	if parsed.Tether.Cny <= 0 {
		return 0, errors.New("coingecko zero cny")
	}
	return parsed.Tether.Cny, nil
}

// IsEpUsdtRateFresh 是否处于有效时效内 (auto 模式)。
// 手动模式 (EpUsdtRateAuto=false) 永远返回 true, 由运营自负。
func IsEpUsdtRateFresh() bool {
	if !setting.EpUsdtRateAuto {
		return true
	}
	if setting.EpUsdtRateUpdatedAt == 0 {
		return false
	}
	return time.Now().Unix()-setting.EpUsdtRateUpdatedAt <= setting.EpUsdtRateStaleSec
}
