package common

import "testing"

func resetSessionCookieSettingsAfterTest(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		SessionCookieSecure = false
		SessionCookieTrustedURLs = nil
	})
}

func TestInitSessionCookieSettingsDefaultsToInsecure(t *testing.T) {
	resetSessionCookieSettingsAfterTest(t)
	t.Setenv("SESSION_COOKIE_SECURE", "")
	t.Setenv("SESSION_COOKIE_TRUSTED_URL", "")

	if err := InitSessionCookieSettings(); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if SessionCookieSecure {
		t.Errorf("expected SessionCookieSecure=false by default")
	}
	if len(SessionCookieTrustedURLs) != 0 {
		t.Errorf("expected empty trusted URLs, got %v", SessionCookieTrustedURLs)
	}
}

func TestInitSessionCookieSettingsRequiresBothEnvVars(t *testing.T) {
	t.Run("secure without trusted url", func(t *testing.T) {
		resetSessionCookieSettingsAfterTest(t)
		t.Setenv("SESSION_COOKIE_SECURE", "true")
		t.Setenv("SESSION_COOKIE_TRUSTED_URL", "")

		if err := InitSessionCookieSettings(); err == nil {
			t.Errorf("expected error when Secure=true without trusted URL")
		}
	})

	t.Run("trusted url without secure", func(t *testing.T) {
		resetSessionCookieSettingsAfterTest(t)
		t.Setenv("SESSION_COOKIE_SECURE", "")
		t.Setenv("SESSION_COOKIE_TRUSTED_URL", "https://example.com")

		if err := InitSessionCookieSettings(); err == nil {
			t.Errorf("expected error when trusted URL set without Secure")
		}
	})
}

func TestInitSessionCookieSettingsRequiresHTTPSURL(t *testing.T) {
	resetSessionCookieSettingsAfterTest(t)
	t.Setenv("SESSION_COOKIE_SECURE", "true")
	t.Setenv("SESSION_COOKIE_TRUSTED_URL", "http://example.com")

	if err := InitSessionCookieSettings(); err == nil {
		t.Errorf("expected error for non-https trusted URL")
	}
}

func TestInitSessionCookieSettingsEnablesSecureCookie(t *testing.T) {
	resetSessionCookieSettingsAfterTest(t)
	t.Setenv("SESSION_COOKIE_SECURE", "true")
	t.Setenv("SESSION_COOKIE_TRUSTED_URL", "https://example.com,https://admin.example.com")

	if err := InitSessionCookieSettings(); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !SessionCookieSecure {
		t.Errorf("expected SessionCookieSecure=true")
	}
	if len(SessionCookieTrustedURLs) != 2 {
		t.Errorf("expected 2 trusted URLs, got %v", SessionCookieTrustedURLs)
	}
}
