package billing_setting

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/setting/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSmokeTestExprRejectsTaskUsage(t *testing.T) {
	for _, expr := range []string{
		`tier("static", u("seconds") * 0.4)`,
		`tier("dynamic", u(header("x-usage-key")) ?? 0.0)`,
	} {
		assert.Error(t, SmokeTestExpr(expr), expr)
	}
	assert.NoError(t, SmokeTestExpr(`len <= 1000 ? tier("a", p * 1 + c * 2) : tier("b", p * 2 + c * 4)`))
}

func TestBillingSettingConcurrentUpdate(t *testing.T) {
	cfg := config.GlobalConfig.Get("billing_setting").(*BillingSetting)
	saved, err := config.ConfigToMap(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, config.UpdateConfigFromMap(cfg, saved)) })

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 500; i++ {
			_ = config.UpdateConfigFromMap(cfg, map[string]string{
				"billing_mode": fmt.Sprintf(`{"m%d":"tiered_expr"}`, i),
				"billing_expr": fmt.Sprintf(`{"m%d":"tier(\"x\", p * 1)"}`, i),
			})
		}
	}()
	for i := 0; i < 500; i++ {
		GetBillingMode(fmt.Sprintf("m%d", i))
		GetBillingExpr("m1")
		GetBillingExprCopy()
	}
	<-done

	// 整体替换：旧键被清除，JSON 往返保持一致
	require.NoError(t, config.UpdateConfigFromMap(cfg, map[string]string{"billing_mode": `{"only":"tiered_expr"}`}))
	assert.Equal(t, BillingModeRatio, GetBillingMode("m499"))
	exported, err := config.ConfigToMap(cfg)
	require.NoError(t, err)
	assert.JSONEq(t, `{"only":"tiered_expr"}`, exported["billing_mode"])
}
