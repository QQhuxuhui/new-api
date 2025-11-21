package common

import (
	"github.com/microcosm-cc/bluemonday"
)

// SanitizeHTML sanitizes HTML content to prevent XSS attacks
// This uses bluemonday's UGC (User Generated Content) policy
// which is a good balance between security and functionality
func SanitizeHTML(html string) string {
	// Use UGC policy which allows common HTML tags and attributes
	// but removes dangerous JavaScript and other XSS vectors
	policy := bluemonday.UGCPolicy()

	// Allow additional safe attributes that might be used in tutorial content
	policy.AllowAttrs("class").Globally()
	policy.AllowAttrs("id").Globally()

	// Allow common table attributes
	policy.AllowAttrs("colspan", "rowspan").OnElements("td", "th")

	// Sanitize and return
	return policy.Sanitize(html)
}

// SanitizeHTMLStrict sanitizes HTML content with a stricter policy
// Use this if you want to be extra cautious
func SanitizeHTMLStrict(html string) string {
	// StrictPolicy only allows very basic formatting tags
	policy := bluemonday.StrictPolicy()
	return policy.Sanitize(html)
}
