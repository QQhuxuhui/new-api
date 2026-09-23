package billing_setting

// Built-in token prices use actual USD per million tokens. Keep new model
// defaults here instead of splitting them across the legacy ratio tables.
//
// dev 只保留上游的文本模型内置价；上游的 gpt-image-2 / gpt-image-2.5-* 依赖 img_cr 与图片张数计费，
// dev 的图片链路（超分降级、按次价格）仍走倍率 / 固定价格，未移植。
var builtinBillingExpr = map[string]string{
	// https://developers.openai.com/api/docs/models/gpt-6-astra
	// Standard pricing; the long-context rates apply to the whole request.
	// Do not infer service-tier discounts from incoming request parameters:
	// channels filter service_tier by default, so it may not reach the upstream.
	"gpt-6-astra": `len <= 272000 ? tier("standard", p * 10 + c * 50 + cr * 1 + cc * 12.5) : tier("long_context", p * 20 + c * 75 + cr * 2 + cc * 25)`,
}
