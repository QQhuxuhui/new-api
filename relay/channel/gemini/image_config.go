package gemini

import (
	"encoding/json"
	"strings"
)

// normalizeImageConfig strips aspect ratio values the Gemini API rejects.
// "auto"/"free" are proxy-side extensions (e.g. adobe2api); the official API
// expresses automatic aspect ratio by omitting the field entirely, and proxies
// treat a missing field the same way, so dropping the value is lossless.
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
		switch strings.ToLower(strings.TrimSpace(ratio)) {
		case "auto", "free":
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
