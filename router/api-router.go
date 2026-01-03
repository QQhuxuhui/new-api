package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"

	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
)

func SetApiRouter(router *gin.Engine) {
	apiRouter := router.Group("/api")
	apiRouter.Use(gzip.Gzip(gzip.DefaultCompression))
	apiRouter.Use(middleware.GlobalAPIRateLimit())
	{
		apiRouter.GET("/setup", controller.GetSetup)
		apiRouter.POST("/setup", controller.PostSetup)
		apiRouter.GET("/status", controller.GetStatus)
		apiRouter.GET("/uptime/status", controller.GetUptimeKumaStatus)
		apiRouter.GET("/models", middleware.UserAuth(), controller.DashboardListModels)
		apiRouter.GET("/status/test", middleware.AdminAuth(), controller.TestStatus)
		apiRouter.GET("/notice", controller.GetNotice)
		apiRouter.GET("/user-agreement", controller.GetUserAgreement)
		apiRouter.GET("/privacy-policy", controller.GetPrivacyPolicy)
		apiRouter.GET("/about", controller.GetAbout)
		//apiRouter.GET("/midjourney", controller.GetMidjourney)
		apiRouter.GET("/home_page_content", controller.GetHomePageContent)
		apiRouter.GET("/pricing", middleware.TryUserAuth(), controller.GetPricing)
		apiRouter.GET("/plan/enabled", controller.GetEnabledPlans) // Public access for plan pricing page
		apiRouter.GET("/verification", middleware.EmailVerificationRateLimit(), middleware.TurnstileCheck(), controller.SendEmailVerification)
		apiRouter.GET("/reset_password", middleware.CriticalRateLimit(), middleware.TurnstileCheck(), controller.SendPasswordResetEmail)
		apiRouter.POST("/user/reset", middleware.CriticalRateLimit(), controller.ResetPassword)
		apiRouter.GET("/oauth/github", middleware.CriticalRateLimit(), controller.GitHubOAuth)
		apiRouter.GET("/oauth/oidc", middleware.CriticalRateLimit(), controller.OidcAuth)
		apiRouter.GET("/oauth/linuxdo", middleware.CriticalRateLimit(), controller.LinuxdoOAuth)
		apiRouter.GET("/oauth/state", middleware.CriticalRateLimit(), controller.GenerateOAuthCode)
		apiRouter.GET("/oauth/wechat", middleware.CriticalRateLimit(), controller.WeChatAuth)
		apiRouter.GET("/oauth/wechat/bind", middleware.CriticalRateLimit(), controller.WeChatBind)
		apiRouter.GET("/oauth/email/bind", middleware.CriticalRateLimit(), controller.EmailBind)
		apiRouter.GET("/oauth/telegram/login", middleware.CriticalRateLimit(), controller.TelegramLogin)
		apiRouter.GET("/oauth/telegram/bind", middleware.CriticalRateLimit(), controller.TelegramBind)
		apiRouter.GET("/ratio_config", middleware.CriticalRateLimit(), controller.GetRatioConfig)

		apiRouter.POST("/stripe/webhook", controller.StripeWebhook)
		apiRouter.POST("/creem/webhook", controller.CreemWebhook)

		// Plan purchase payment callback (no auth required - handled by payment gateway)
		apiRouter.GET("/plan/purchase/epay/notify", controller.EpayPlanOrderNotify)

		// Topup order payment callback (no auth required - handled by payment gateway)
		apiRouter.GET("/user/topup/order/epay/notify", controller.EpayTopupOrderNotify)

		// Universal secure verification routes
		apiRouter.POST("/verify", middleware.UserAuth(), middleware.CriticalRateLimit(), controller.UniversalVerify)
		apiRouter.GET("/verify/status", middleware.UserAuth(), controller.GetVerificationStatus)

		userRoute := apiRouter.Group("/user")
		{
			userRoute.POST("/register", middleware.CriticalRateLimit(), middleware.TurnstileCheck(), controller.Register)
			userRoute.POST("/login", middleware.CriticalRateLimit(), middleware.TurnstileCheck(), controller.Login)
			userRoute.POST("/login/2fa", middleware.CriticalRateLimit(), controller.Verify2FALogin)
			userRoute.POST("/passkey/login/begin", middleware.CriticalRateLimit(), controller.PasskeyLoginBegin)
			userRoute.POST("/passkey/login/finish", middleware.CriticalRateLimit(), controller.PasskeyLoginFinish)
			//userRoute.POST("/tokenlog", middleware.CriticalRateLimit(), controller.TokenLog)
			userRoute.GET("/logout", controller.Logout)
			userRoute.GET("/epay/notify", controller.EpayNotify)
			userRoute.GET("/groups", controller.GetUserGroups)
			userRoute.GET("/topup/info", controller.GetTopUpInfo) // Public access for pricing page

			selfRoute := userRoute.Group("/")
			selfRoute.Use(middleware.UserAuth())
			{
				selfRoute.GET("/self/groups", controller.GetUserGroups)
				selfRoute.GET("/self", controller.GetSelf)
				selfRoute.GET("/models", controller.GetUserModels)
				selfRoute.PUT("/self", controller.UpdateSelf)
				selfRoute.DELETE("/self", controller.DeleteSelf)
				selfRoute.GET("/token", controller.GenerateAccessToken)
				selfRoute.GET("/passkey", controller.PasskeyStatus)
				selfRoute.POST("/passkey/register/begin", controller.PasskeyRegisterBegin)
				selfRoute.POST("/passkey/register/finish", controller.PasskeyRegisterFinish)
				selfRoute.POST("/passkey/verify/begin", controller.PasskeyVerifyBegin)
				selfRoute.POST("/passkey/verify/finish", controller.PasskeyVerifyFinish)
				selfRoute.DELETE("/passkey", controller.PasskeyDelete)
				selfRoute.GET("/aff", controller.GetAffCode)
				selfRoute.GET("/topup/self", controller.GetUserTopUps)
				selfRoute.POST("/topup", middleware.CriticalRateLimit(), controller.TopUp)
				selfRoute.POST("/pay", middleware.CriticalRateLimit(), controller.RequestEpay)
				selfRoute.POST("/amount", controller.RequestAmount)
				selfRoute.POST("/stripe/pay", middleware.CriticalRateLimit(), controller.RequestStripePay)
				selfRoute.POST("/stripe/amount", controller.RequestStripeAmount)
				selfRoute.POST("/creem/pay", middleware.CriticalRateLimit(), controller.RequestCreemPay)
				selfRoute.POST("/aff_transfer", controller.TransferAffQuota)
				selfRoute.PUT("/setting", controller.UpdateUserSetting)

				// 2FA routes
				selfRoute.GET("/2fa/status", controller.Get2FAStatus)
				selfRoute.POST("/2fa/setup", controller.Setup2FA)
				selfRoute.POST("/2fa/enable", controller.Enable2FA)
				selfRoute.POST("/2fa/disable", controller.Disable2FA)
				selfRoute.POST("/2fa/backup_codes", controller.RegenerateBackupCodes)

				// Notification routes
				selfRoute.GET("/notifications", controller.GetMyNotifications)
				selfRoute.GET("/notifications/unread-count", controller.GetUnreadNotificationCount)
				selfRoute.POST("/notifications/:id/read", controller.MarkNotificationAsRead)
				selfRoute.POST("/notifications/read-all", controller.MarkAllNotificationsAsRead)

				// Plan purchase routes (user)
				selfRoute.POST("/plan/purchase/create", controller.CreatePlanOrder)
				selfRoute.POST("/plan/purchase/pay", middleware.CriticalRateLimit(), controller.PayPlanOrder)
				selfRoute.GET("/plan/purchase/my-orders", controller.GetMyPlanOrders)
				selfRoute.POST("/plan/purchase/cancel", controller.CancelPlanOrder)

				// Topup order routes (user) - for pricing page pay-as-you-go
				selfRoute.POST("/topup/order/create", controller.CreateTopupOrder)
				selfRoute.GET("/topup/order/:id", controller.GetTopupOrderDetail)
				selfRoute.POST("/topup/order/pay", middleware.CriticalRateLimit(), controller.PayTopupOrder)
				selfRoute.GET("/topup/order/my-orders", controller.GetMyTopupOrders)
				selfRoute.POST("/topup/order/cancel", controller.CancelTopupOrder)
			}

			adminRoute := userRoute.Group("/")
			adminRoute.Use(middleware.AdminAuth())
			{
				adminRoute.GET("/", controller.GetAllUsers)
				adminRoute.GET("/topup", controller.GetAllTopUps)
				adminRoute.POST("/topup/complete", controller.AdminCompleteTopUp)

				// Plan order management (admin)
				adminRoute.GET("/plan-orders", controller.GetAllPlanOrders)
				adminRoute.GET("/plan-orders/:id", controller.GetPlanOrderDetail)
				adminRoute.POST("/plan-orders/:id/complete", controller.ManualCompletePlanOrder)
				adminRoute.POST("/plan-orders/:id/cancel", controller.AdminCancelPlanOrder)
				adminRoute.DELETE("/plan-orders/:id", controller.DeletePlanOrder)

				adminRoute.GET("/search", controller.SearchUsers)
				adminRoute.GET("/:id", controller.GetUser)
				adminRoute.POST("/", controller.CreateUser)
				adminRoute.POST("/manage", controller.ManageUser)
				adminRoute.PUT("/", controller.UpdateUser)
				adminRoute.DELETE("/:id", controller.DeleteUser)
				adminRoute.DELETE("/:id/reset_passkey", controller.AdminResetPasskey)

				// Admin 2FA routes
				adminRoute.GET("/2fa/stats", controller.Admin2FAStats)
				adminRoute.DELETE("/:id/2fa", controller.AdminDisable2FA)
			}
		}
		optionRoute := apiRouter.Group("/option")
		optionRoute.Use(middleware.RootAuth())
		{
			optionRoute.GET("/", controller.GetOptions)
			optionRoute.PUT("/", controller.UpdateOption)
			optionRoute.POST("/rest_model_ratio", controller.ResetModelRatio)
			optionRoute.POST("/migrate_console_setting", controller.MigrateConsoleSetting) // 用于迁移检测的旧键，下个版本会删除
		}
		ratioSyncRoute := apiRouter.Group("/ratio_sync")
		ratioSyncRoute.Use(middleware.RootAuth())
		{
			ratioSyncRoute.GET("/channels", controller.GetSyncableChannels)
			ratioSyncRoute.POST("/fetch", controller.FetchUpstreamRatios)
		}
		channelRoute := apiRouter.Group("/channel")
		channelRoute.Use(middleware.AdminAuth())
		{
			channelRoute.GET("/", controller.GetAllChannels)
			channelRoute.GET("/search", controller.SearchChannels)
			channelRoute.GET("/models", controller.ChannelListModels)
			channelRoute.GET("/models_enabled", controller.EnabledListModels)
			channelRoute.GET("/groups", controller.GetChannelGroups) // Get all distinct channel groups
			channelRoute.GET("/:id", controller.GetChannel)
			channelRoute.GET("/:id/concurrency", controller.GetChannelConcurrency)
			channelRoute.GET("/:id/health", controller.GetChannelHealth)
			channelRoute.POST("/:id/health/reset", controller.ResetChannelHealth)
			channelRoute.POST("/:id/key", middleware.RootAuth(), middleware.CriticalRateLimit(), middleware.DisableCache(), middleware.SecureVerificationRequired(), controller.GetChannelKey)
			channelRoute.GET("/test", controller.TestAllChannels)
			channelRoute.GET("/test/:id", controller.TestChannel)
			channelRoute.GET("/update_balance", controller.UpdateAllChannelsBalance)
			channelRoute.GET("/update_balance/:id", controller.UpdateChannelBalance)
			channelRoute.POST("/set_balance/:id", controller.SetChannelBalanceManually)
			channelRoute.GET("/health", controller.GetAllChannelsHealth)
			channelRoute.POST("/", controller.AddChannel)
			channelRoute.PUT("/", controller.UpdateChannel)
			channelRoute.DELETE("/disabled", controller.DeleteDisabledChannel)
			channelRoute.POST("/tag/disabled", controller.DisableTagChannels)
			channelRoute.POST("/tag/enabled", controller.EnableTagChannels)
			channelRoute.PUT("/tag", controller.EditTagChannels)
			channelRoute.DELETE("/:id", controller.DeleteChannel)
			channelRoute.POST("/batch", controller.DeleteChannelBatch)
			channelRoute.POST("/fix", controller.FixChannelsAbilities)
			channelRoute.GET("/fetch_models/:id", controller.FetchUpstreamModels)
			channelRoute.POST("/fetch_models", controller.FetchModels)
			channelRoute.POST("/batch/tag", controller.BatchSetChannelTag)
			channelRoute.GET("/tag/models", controller.GetTagModels)
			channelRoute.POST("/copy/:id", controller.CopyChannel)
			channelRoute.POST("/multi_key/manage", controller.ManageMultiKeys)
		}

		// Channel failover rule management (admin)
		disableRuleRoute := apiRouter.Group("/channel/disable-rules")
		disableRuleRoute.Use(middleware.AdminAuth())
		{
			disableRuleRoute.GET("/", controller.GetDisableRules)
			disableRuleRoute.POST("/", controller.CreateDisableRule)
			disableRuleRoute.PUT("/:id", controller.UpdateDisableRule)
			disableRuleRoute.DELETE("/:id", controller.DeleteDisableRule)
			disableRuleRoute.POST("/test", controller.TestDisableRule)
			disableRuleRoute.POST("/refresh-cache", controller.RefreshDisableRulesCache)
		}
		tokenRoute := apiRouter.Group("/token")
		tokenRoute.Use(middleware.UserAuth())
		{
			tokenRoute.GET("/", controller.GetAllTokens)
			tokenRoute.GET("/search", controller.SearchTokens)
			tokenRoute.GET("/:id", controller.GetToken)
			tokenRoute.POST("/", controller.AddToken)
			tokenRoute.PUT("/", controller.UpdateToken)
			tokenRoute.DELETE("/:id", controller.DeleteToken)
			tokenRoute.POST("/batch", controller.DeleteTokenBatch)
		}

		usageRoute := apiRouter.Group("/usage")
		usageRoute.Use(middleware.CriticalRateLimit())
		{
			tokenUsageRoute := usageRoute.Group("/token")
			tokenUsageRoute.Use(middleware.TokenAuth())
			{
				tokenUsageRoute.GET("/", controller.GetTokenUsage)
			}
		}

		redemptionRoute := apiRouter.Group("/redemption")
		redemptionRoute.Use(middleware.AdminAuth())
		{
			redemptionRoute.GET("/", controller.GetAllRedemptions)
			redemptionRoute.GET("/search", controller.SearchRedemptions)
			redemptionRoute.GET("/:id", controller.GetRedemption)
			redemptionRoute.POST("/", controller.AddRedemption)
			redemptionRoute.PUT("/", controller.UpdateRedemption)
			redemptionRoute.DELETE("/invalid", controller.DeleteInvalidRedemption)
			redemptionRoute.DELETE("/:id", controller.DeleteRedemption)
		}
		logRoute := apiRouter.Group("/log")
		logRoute.GET("/", middleware.AdminAuth(), controller.GetAllLogs)
		logRoute.DELETE("/", middleware.AdminAuth(), controller.DeleteHistoryLogs)
		logRoute.GET("/stat", middleware.AdminAuth(), controller.GetLogsStat)
		logRoute.GET("/self/stat", middleware.UserAuth(), controller.GetLogsSelfStat)
		logRoute.GET("/search", middleware.AdminAuth(), controller.SearchAllLogs)
		logRoute.GET("/self", middleware.UserAuth(), controller.GetUserLogs)
		logRoute.GET("/self/search", middleware.UserAuth(), controller.SearchUserLogs)
		logRoute.GET("/plans", middleware.UserAuth(), controller.GetUserLogPlans)

		dataRoute := apiRouter.Group("/data")
		dataRoute.GET("/", middleware.AdminAuth(), controller.GetAllQuotaDates)
		dataRoute.GET("/self", middleware.UserAuth(), controller.GetUserQuotaDates)

		// Analytics routes (admin only)
		analyticsRoute := apiRouter.Group("/admin/analytics")
		analyticsRoute.Use(middleware.AdminAuth())
		{
			analyticsRoute.GET("/user-overview", controller.GetUserOverview)
			analyticsRoute.GET("/active-users", controller.GetActiveUsers)
			analyticsRoute.GET("/consumption-ranking", controller.GetConsumptionRanking)
			analyticsRoute.GET("/consumption-trend", controller.GetConsumptionTrend)
			analyticsRoute.GET("/model-usage", controller.GetModelUsage)
			analyticsRoute.GET("/behavior-patterns", controller.GetBehaviorPatterns)
			analyticsRoute.GET("/risk-indicators", controller.GetRiskIndicators)
			analyticsRoute.GET("/user-balance-analysis", controller.GetUserBalanceAnalysis)
			analyticsRoute.GET("/user-consumption-detail/:user_id", controller.GetUserConsumptionDetail)
			analyticsRoute.GET("/user-daily-consumption-trend", controller.GetUserDailyConsumptionTrend)
			analyticsRoute.GET("/export", controller.ExportAnalyticsData)
		// Cost analytics endpoints
		analyticsRoute.GET("/channel-cost-analysis", controller.GetChannelCostAnalysis)
		analyticsRoute.GET("/cost-trend", controller.GetCostTrend)
		analyticsRoute.GET("/model-cost-analysis", controller.GetModelCostAnalysis)
		// Quota-based analytics endpoints (not requiring model_price)
		analyticsRoute.GET("/channel-quota-analysis", controller.GetChannelQuotaAnalysis)
		analyticsRoute.GET("/quota-trend", controller.GetQuotaTrend)
		analyticsRoute.GET("/channel-daily-quota-trend", controller.GetChannelDailyQuotaTrend)
		// Plan usage analytics endpoints
		analyticsRoute.GET("/plan-usage/overview", controller.GetPlanUsageOverview)
		analyticsRoute.GET("/plan-usage/list", controller.GetPlanUsageList)
		analyticsRoute.GET("/plan-usage/type-distribution", controller.GetPlanTypeDistribution)
		analyticsRoute.GET("/plan-usage/consumption-ranking", controller.GetPlanConsumptionRanking)
		analyticsRoute.GET("/plan-usage/user-daily", controller.GetUserDailyUsage)
		}

		logRoute.Use(middleware.CORS())
		{
			logRoute.GET("/token", controller.GetLogByKey)
		}
		groupRoute := apiRouter.Group("/group")
		groupRoute.Use(middleware.AdminAuth())
		{
			groupRoute.GET("/", controller.GetGroups)
			groupRoute.GET("/tree", controller.GetGroupTree)
			groupRoute.GET("/selectable", controller.GetSelectableGroups)
			groupRoute.GET("/all", controller.GetAllGroupsWithTree)
		}

		prefillGroupRoute := apiRouter.Group("/prefill_group")
		prefillGroupRoute.Use(middleware.AdminAuth())
		{
			prefillGroupRoute.GET("/", controller.GetPrefillGroups)
			prefillGroupRoute.POST("/", controller.CreatePrefillGroup)
			prefillGroupRoute.PUT("/", controller.UpdatePrefillGroup)
			prefillGroupRoute.DELETE("/:id", controller.DeletePrefillGroup)
		}

		mjRoute := apiRouter.Group("/mj")
		mjRoute.GET("/self", middleware.UserAuth(), controller.GetUserMidjourney)
		mjRoute.GET("/", middleware.AdminAuth(), controller.GetAllMidjourney)

		taskRoute := apiRouter.Group("/task")
		{
			taskRoute.GET("/self", middleware.UserAuth(), controller.GetUserTask)
			taskRoute.GET("/", middleware.AdminAuth(), controller.GetAllTask)
		}

		vendorRoute := apiRouter.Group("/vendors")
		vendorRoute.Use(middleware.AdminAuth())
		{
			vendorRoute.GET("/", controller.GetAllVendors)
			vendorRoute.GET("/search", controller.SearchVendors)
			vendorRoute.GET("/:id", controller.GetVendorMeta)
			vendorRoute.POST("/", controller.CreateVendorMeta)
			vendorRoute.PUT("/", controller.UpdateVendorMeta)
			vendorRoute.DELETE("/:id", controller.DeleteVendorMeta)
		}

		modelsRoute := apiRouter.Group("/models")
		modelsRoute.Use(middleware.AdminAuth())
		{
			modelsRoute.GET("/sync_upstream/preview", controller.SyncUpstreamPreview)
			modelsRoute.POST("/sync_upstream", controller.SyncUpstreamModels)
			modelsRoute.GET("/missing", controller.GetMissingModels)
			modelsRoute.GET("/", controller.GetAllModelsMeta)
			modelsRoute.GET("/search", controller.SearchModelsMeta)
			modelsRoute.GET("/:id", controller.GetModelMeta)
			modelsRoute.POST("/", controller.CreateModelMeta)
			modelsRoute.PUT("/", controller.UpdateModelMeta)
			modelsRoute.DELETE("/:id", controller.DeleteModelMeta)
		}

		// Plan management routes (admin)
		planRoute := apiRouter.Group("/plan")
		planRoute.Use(middleware.AdminAuth())
		{
			planRoute.GET("/", controller.GetAllPlans)
			planRoute.GET("/search", controller.SearchPlans)
			planRoute.GET("/:id", controller.GetPlan)
			planRoute.POST("/", controller.AddPlan)
			planRoute.PUT("/", controller.UpdatePlan)
			planRoute.DELETE("/:id", controller.DeletePlan)
			planRoute.PUT("/:id/status", controller.UpdatePlanStatus)
			planRoute.GET("/:id/sync_status", controller.GetPlanSyncStatus)
			planRoute.POST("/:id/retry_sync", controller.RetryPlanSync)
		}

		// User plan management routes (admin)
		userPlanAdminRoute := apiRouter.Group("/user_plan")
		userPlanAdminRoute.Use(middleware.AdminAuth())
		{
			userPlanAdminRoute.GET("/", controller.GetAllUserPlans)
			userPlanAdminRoute.GET("/plan/:plan_id", controller.GetUserPlansByPlan)
			userPlanAdminRoute.GET("/user/:user_id", controller.GetUserPlansForUser)
			userPlanAdminRoute.GET("/:id", controller.GetUserPlan)
			userPlanAdminRoute.PUT("/:id", controller.AdminUpdateUserPlan)                          // Update user plan (quota, expiry, daily limit override, etc.)
			userPlanAdminRoute.POST("/assign", controller.AdminAssignPlan)
			userPlanAdminRoute.POST("/remove", controller.AdminRemovePlan)
			userPlanAdminRoute.DELETE("/:id", controller.AdminRemovePlanById) // Remove user plan by ID (supports deleted plan templates)
			userPlanAdminRoute.PUT("/:id/permissions", controller.AdminUpdateUserPlanPermissions)
			userPlanAdminRoute.POST("/force_switch", controller.AdminForceSwitch)
			userPlanAdminRoute.POST("/:id/lock", controller.AdminLockUserPlan)
			userPlanAdminRoute.POST("/:id/unlock", controller.AdminUnlockUserPlan)
			userPlanAdminRoute.PUT("/:id/quota", controller.AdminAdjustQuota)
			userPlanAdminRoute.POST("/:id/add_quota", controller.AdminAddQuota)
			userPlanAdminRoute.DELETE("/:id/daily_quota_override", controller.AdminClearDailyQuotaOverride) // Clear daily quota override
			userPlanAdminRoute.GET("/:id/quota-status", controller.GetUserPlanQuotaStatus)
			// Queue management
			userPlanAdminRoute.POST("/user/:user_id/queue/reorder", controller.AdminReorderQueue)
			userPlanAdminRoute.DELETE("/:id/queue", controller.AdminRemoveFromQueue)
			userPlanAdminRoute.POST("/:id/revoke", controller.AdminRevokePlan)
		}

		// Daily pool management routes (admin)
		dailyPoolAdminRoute := apiRouter.Group("/daily_pool")
		dailyPoolAdminRoute.Use(middleware.AdminAuth())
		{
			dailyPoolAdminRoute.GET("/user/:user_id", controller.AdminGetUserDailyPool)
			dailyPoolAdminRoute.PUT("/user/:user_id", controller.AdminAdjustDailyPool)
			dailyPoolAdminRoute.POST("/user/:user_id", controller.AdminCreateDailyPool)
		}

		// Refund management routes (admin)
		refundAdminRoute := apiRouter.Group("/refund")
		refundAdminRoute.Use(middleware.AdminAuth())
		{
			refundAdminRoute.GET("/pending", controller.AdminGetPendingRefunds)
			refundAdminRoute.POST("/:id/approve", controller.AdminApproveRefund)
			refundAdminRoute.POST("/:id/reject", controller.AdminRejectRefund)
		}

		// Asset snapshot routes (admin)
		snapshotAdminRoute := apiRouter.Group("/asset_snapshot")
		snapshotAdminRoute.Use(middleware.AdminAuth())
		{
			snapshotAdminRoute.GET("/user/:user_id", controller.AdminGetAssetSnapshots)
			snapshotAdminRoute.POST("/:id/restore", controller.AdminRestoreFromSnapshot)
		}

		// Admin plan operation logs
		planLogRoute := apiRouter.Group("/plan_log")
		planLogRoute.Use(middleware.AdminAuth())
		{
			planLogRoute.GET("/", controller.AdminGetPlanOperationLogs)
		}

		// User plan routes (user)
		userPlanRoute := apiRouter.Group("/my_plans")
		userPlanRoute.Use(middleware.UserAuth())
		{
			userPlanRoute.GET("/", controller.GetMyPlans)
			userPlanRoute.POST("/switch", controller.UserSwitchPlan)
			userPlanRoute.PUT("/:id/auto_switch", controller.UserToggleAutoSwitch)
			userPlanRoute.GET("/quota-status", controller.GetCurrentPlanQuotaStatus)
			userPlanRoute.GET("/queue", controller.UserGetQueuedPlans)
			userPlanRoute.GET("/billing-status", controller.UserGetBillingStatus)
			userPlanRoute.POST("/:id/refund", controller.UserRequestRefund)
			userPlanRoute.GET("/refund-history", controller.UserGetRefundHistory)
		}

		// Plan migration routes (root admin only)
		migrationRoute := apiRouter.Group("/plan_migration")
		migrationRoute.Use(middleware.RootAuth())
		{
			migrationRoute.GET("/status", controller.GetMigrationStatus)
			migrationRoute.POST("/run", controller.RunMigration)
			migrationRoute.POST("/rollback", controller.RollbackMigration)
			migrationRoute.POST("/user", controller.MigrateSingleUser)
		}
	}
}
