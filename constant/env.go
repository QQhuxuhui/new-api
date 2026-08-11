package constant

var StreamingTimeout int

// MaxStreamTimeoutSeconds 渠道级流式超时(stream_timeout_seconds)允许的最大值
// （7 天）。超过该值保存时被拒绝；如需不限制请显式配置 0（永不超时）。
const MaxStreamTimeoutSeconds = 7 * 24 * 60 * 60
var DifyDebug bool
var MaxFileDownloadMB int
var ForceStreamOption bool
var GetMediaToken bool
var GetMediaTokenNotStream bool
var UpdateTask bool
var AzureDefaultAPIVersion string
var GeminiVisionMaxImageNum int
var NotifyLimitCount int
var NotificationLimitDurationMinute int
var GenerateDefaultToken bool
var ErrorLogEnabled bool
var TaskQueryLimit int
var TaskTimeoutMinutes int

// temporary variable for sora patch, will be removed in future
var TaskPricePatches []string
