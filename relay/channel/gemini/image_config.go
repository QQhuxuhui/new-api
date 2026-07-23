package gemini

import (
	"encoding/json"
	"strings"
)

// supportedGeminiAspectRatios is the full set the Gemini image API accepts
// (flash superset; pro accepts the 10 without 1:4/1:8/4:1/8:1). Any other
// value — "auto"/"free" proxy extensions, non-standard ratios like "2:1",
// pixel sizes, or empty — is rejected upstream with a hard error.
var supportedGeminiAspectRatios = map[string]bool{
	"1:1": true, "2:3": true, "3:2": true, "3:4": true, "4:3": true,
	"4:5": true, "5:4": true, "9:16": true, "16:9": true, "21:9": true,
	"1:4": true, "1:8": true, "4:1": true, "8:1": true,
}

// normalizeImageConfig drops any aspect_ratio the Gemini API would reject.
// The official API expresses "automatic" by omitting the field, and upstream
// proxies treat a missing field the same way, so dropping an unsupported value
// is lossless and keeps the request from failing on strict passthrough
// channels (some upstreams tolerate odd ratios, others reject them outright).
func normalizeImageConfig(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return raw
	}
	var config map[string]json.RawMessage
	if err := json.Unmarshal(raw, &config); err != nil {
		return raw
	}
	changed := false
	for _, key := range []string{"aspectRatio", "aspect_ratio"} {
		value, ok := config[key]
		if !ok {
			continue
		}
		var ratio string
		if err := json.Unmarshal(value, &ratio); err != nil {
			continue
		}
		if !supportedGeminiAspectRatios[strings.TrimSpace(ratio)] {
			delete(config, key)
			changed = true
		}
	}
	if !changed {
		return raw
	}
	if len(config) == 0 {
		return nil
	}
	normalized, err := json.Marshal(config)
	if err != nil {
		return raw
	}
	return normalized
}
