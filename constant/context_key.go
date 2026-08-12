package constant

type ContextKey string

const (
	ContextKeyTokenCountMeta ContextKey = "token_count_meta"
	ContextKeyPromptTokens   ContextKey = "prompt_tokens"

	ContextKeyOriginalModel    ContextKey = "original_model"
	ContextKeyRequestStartTime ContextKey = "request_start_time"

	/* token related keys */
	ContextKeyTokenUnlimited         ContextKey = "token_unlimited_quota"
	ContextKeyTokenKey               ContextKey = "token_key"
	ContextKeyTokenId                ContextKey = "token_id"
	ContextKeyTokenGroup             ContextKey = "token_group"
	ContextKeyTokenSpecificChannelId ContextKey = "specific_channel_id"
	ContextKeyTokenModelLimitEnabled ContextKey = "token_model_limit_enabled"
	ContextKeyTokenModelLimit        ContextKey = "token_model_limit"

	/* channel related keys */
	ContextKeyChannelId                ContextKey = "channel_id"
	ContextKeyChannelName              ContextKey = "channel_name"
	ContextKeyChannelCreateTime        ContextKey = "channel_create_time"
	ContextKeyChannelBaseUrl           ContextKey = "base_url"
	ContextKeyChannelType              ContextKey = "channel_type"
	ContextKeyChannelSetting           ContextKey = "channel_setting"
	ContextKeyChannelOtherSetting      ContextKey = "channel_other_setting"
	ContextKeyChannelParamOverride     ContextKey = "param_override"
	ContextKeyChannelHeaderOverride    ContextKey = "header_override"
	ContextKeyChannelOrganization      ContextKey = "channel_organization"
	ContextKeyChannelAutoBan           ContextKey = "auto_ban"
	ContextKeyChannelModelMapping      ContextKey = "model_mapping"
	ContextKeyChannelStatusCodeMapping ContextKey = "status_code_mapping"
	ContextKeyChannelIsMultiKey        ContextKey = "channel_is_multi_key"
	ContextKeyChannelMultiKeyIndex     ContextKey = "channel_multi_key_index"
	ContextKeyChannelKey               ContextKey = "channel_key"
	ContextKeyChannelRatio             ContextKey = "channel_ratio"
	ContextKeyChannelModelRatio        ContextKey = "channel_model_ratio"    // float64: 渠道模型倍率
	ContextKeyChannelPriorityIndex     ContextKey = "channel_priority_index" // int: 中间件选择渠道时的优先级索引，用于重试时继续遍历
	ContextKeyChannelTest              ContextKey = "is_channel_test"        // bool: 本请求来自渠道测试，没有真实客户端请求头可透传

	/* user related keys */
	ContextKeyUserId             ContextKey = "id"
	ContextKeyUserSetting        ContextKey = "user_setting"
	ContextKeyUserQuota          ContextKey = "user_quota"
	ContextKeyUserStatus         ContextKey = "user_status"
	ContextKeyUserEmail          ContextKey = "user_email"
	ContextKeyUserGroup          ContextKey = "user_group"
	ContextKeyUsingGroup         ContextKey = "group"
	ContextKeyUserName           ContextKey = "username"
	ContextKeyUserMaxConcurrency ContextKey = "user_max_concurrency"

	ContextKeySystemPromptOverride ContextKey = "system_prompt_override"

	/* plan related keys */
	ContextKeyPlanId         ContextKey = "plan_id"          // int: selected plan ID
	ContextKeyUserPlanId     ContextKey = "user_plan_id"     // int: user plan assignment ID
	ContextKeyPlanGroup      ContextKey = "plan_group"       // string: channel group from plan (single, for compatibility)
	ContextKeyPlanGroups     ContextKey = "plan_groups"      // []string: all channel groups from plan (multi-group support)
	ContextKeyPlanName       ContextKey = "plan_name"        // string: plan name for logging
	ContextKeyPlanAutoSwitch ContextKey = "plan_auto_switch" // bool: if auto-switch occurred

	ContextKeyClientErrorFlag   ContextKey = "client_error_rule_matched" // bool: current request matched a client-classified custom rule
	ContextKeyReturnImmediately ContextKey = "return_immediately"        // bool: matched client rule requires immediate return

	ContextKeyErrorCaptureDone ContextKey = "error_capture_done" // bool: 本请求已捕获过错误请求体，避免重试重复捕获

	ContextKeyPayloadWritten ContextKey = "relay_payload_written" // bool: 已有业务字节写回客户端，跨渠道重试/降级必须停止
	ContextKeyPingWritten    ContextKey = "relay_ping_written"    // bool: 仅写出过 SSE ping 注释（不算 payload，可安全重放）

	ContextKeyWarningChannelSkipped ContextKey = "warning_channel_skipped" // bool: 本请求曾有渠道仅因 warning 掷骰被跳过，优先级耗尽时值得关骰补扫

	ContextKeyMidStreamTimeout ContextKey = "mid_stream_timeout" // bool: 流已输出部分内容后发生空闲超时；外层改记渠道失败，handler 跳过伪造的正常收尾

	ContextKeyStreamIdleTimeoutState ContextKey = "stream_idle_timeout_state" // *idleTimeoutBody: 原始扫描循环的空闲超时状态；定时器只改其原子字段，不写 Context
)
