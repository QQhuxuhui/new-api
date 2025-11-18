/**
 * Analytics tracking helper
 * Tracks user events for analytics and monitoring
 */

/**
 * Track an analytics event
 * @param {string} eventName - Name of the event to track
 * @param {object} properties - Event properties/metadata
 */
export const trackEvent = (eventName, properties = {}) => {
  try {
    // Add timestamp
    const eventData = {
      event: eventName,
      timestamp: new Date().toISOString(),
      ...properties,
    };

    // Log to console in development
    if (import.meta.env.DEV) {
      console.log('[Analytics]', eventData);
    }

    // Send to analytics platform (can be extended later)
    // Example: window.gtag?.('event', eventName, properties);
    // Example: window.mixpanel?.track(eventName, properties);

    // Store in localStorage for debugging (optional)
    if (typeof window !== 'undefined' && window.localStorage) {
      const events = JSON.parse(
        localStorage.getItem('analytics_events') || '[]',
      );
      events.push(eventData);
      // Keep only last 100 events
      if (events.length > 100) {
        events.shift();
      }
      localStorage.setItem('analytics_events', JSON.stringify(events));
    }
  } catch (error) {
    console.error('Failed to track event:', error);
  }
};

/**
 * Token creation analytics events
 */
export const TokenAnalytics = {
  // Track mode selection (quick vs advanced)
  trackModeSelected: (mode) => {
    trackEvent('token_create_mode_selected', {
      mode, // 'quick' or 'advanced'
    });
  },

  // Track token type selection in quick create
  trackTypeSelected: (type) => {
    trackEvent('quick_create_type_selected', {
      type, // 'claude-code' or 'codex'
    });
  },

  // Track quick create success
  trackQuickCreateSuccess: (type, timeSpent) => {
    trackEvent('quick_create_success', {
      type,
      time_spent: timeSpent, // in seconds
    });
  },

  // Track quick create failure
  trackQuickCreateFailed: (type, errorMessage) => {
    trackEvent('quick_create_failed', {
      type,
      error_message: errorMessage,
    });
  },

  // Track switch from quick create to advanced mode
  trackSwitchedToAdvanced: (fromStep) => {
    trackEvent('switched_to_advanced', {
      from_step: fromStep, // 1 or 2
    });
  },

  // Track token key copied
  trackTokenKeyCopied: () => {
    trackEvent('token_key_copied', {});
  },
};

/**
 * Onboarding wizard analytics events
 */
export const OnboardingAnalytics = {
  // Track wizard start
  trackStarted: (autoStart) => {
    trackEvent('onboarding_started', {
      auto_start: autoStart,
    });
  },

  // Track step completion
  trackStepCompleted: (step, timeSpent, data = {}) => {
    trackEvent('onboarding_step_completed', {
      step,
      time_spent: timeSpent,
      ...data,
    });
  },

  // Track step skip
  trackStepSkipped: (step, timeSpent) => {
    trackEvent('onboarding_step_skipped', {
      step,
      time_spent: timeSpent,
    });
  },

  // Track wizard completion
  trackCompleted: (timeSpent, completionRate, createdToken, toppedUp) => {
    trackEvent('onboarding_completed', {
      time_spent: timeSpent,
      completion_rate: completionRate,
      created_token: createdToken,
      topped_up: toppedUp,
    });
  },

  // Track wizard close
  trackClosed: (step, completionRate) => {
    trackEvent('onboarding_closed', {
      step,
      completion_rate: completionRate,
    });
  },

  // Track redemption code used in onboarding
  trackRedemptionCodeUsed: () => {
    trackEvent('onboarding_redemption_code_used', {});
  },

  // Track token created in onboarding
  trackTokenCreated: () => {
    trackEvent('onboarding_token_created', {});
  },

  // Track code snippet copied in onboarding
  trackCodeCopied: (language) => {
    trackEvent('onboarding_code_copied', {
      language,
    });
  },
};
