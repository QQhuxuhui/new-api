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

	/* user related keys */
	ContextKeyUserId      ContextKey = "id"
	ContextKeyUserSetting ContextKey = "user_setting"
	ContextKeyUserQuota   ContextKey = "user_quota"
	ContextKeyUserStatus  ContextKey = "user_status"
	ContextKeyUserEmail   ContextKey = "user_email"
	ContextKeyUserGroup   ContextKey = "user_group"
	ContextKeyUsingGroup  ContextKey = "group"
	ContextKeyUserName    ContextKey = "username"

	ContextKeySystemPromptOverride ContextKey = "system_prompt_override"

	/* sticky session keys */
	ContextKeyStickySession     ContextKey = "sticky_session"      // bool: enabled for this token
	ContextKeyStickySessionTTL  ContextKey = "sticky_session_ttl"  // int: TTL in seconds
	ContextKeyStickySessionUsed ContextKey = "sticky_session_used" // bool: used existing binding
	ContextKeyStickySessionNew  ContextKey = "sticky_session_new"  // bool: created new binding

	/* client restriction keys */
	ContextKeyUserAgent ContextKey = "user_agent" // string: client User-Agent

	/* plan related keys */
	ContextKeyPlanId         ContextKey = "plan_id"          // int: selected plan ID
	ContextKeyUserPlanId     ContextKey = "user_plan_id"     // int: user plan assignment ID
	ContextKeyPlanGroup      ContextKey = "plan_group"       // string: channel group from plan
	ContextKeyPlanName       ContextKey = "plan_name"        // string: plan name for logging
	ContextKeyPlanAutoSwitch ContextKey = "plan_auto_switch" // bool: if auto-switch occurred
)
