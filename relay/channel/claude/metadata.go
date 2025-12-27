package claude

import (
	"encoding/json"
)

// masqueradeMetadata sets metadata.user_id to the fixed masquerade value while
// preserving other metadata fields when possible. It returns the masked
// metadata payload and the original user_id (if present).
func masqueradeMetadata(raw json.RawMessage) (json.RawMessage, string) {
	originalUserID := "<empty>"

	meta := make(map[string]any)
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &meta); err == nil {
			if uid, ok := meta["user_id"].(string); ok && uid != "" {
				originalUserID = uid
			}
		} else {
			meta = make(map[string]any)
		}
	}

	// Overwrite/insert masked user_id
	meta["user_id"] = MasqueradeUserID

	masked, err := json.Marshal(meta)
	if err != nil {
		return json.RawMessage(`{"user_id":"` + MasqueradeUserID + `"}`), originalUserID
	}

	return masked, originalUserID
}

// MasqueradeMetadataInBody updates the top-level metadata field in a Claude
// request body, preserving other top-level fields. It returns the updated body
// and the original user_id (if any) from metadata.
func MasqueradeMetadataInBody(body []byte) ([]byte, string) {
	payload := make(map[string]json.RawMessage)
	if err := json.Unmarshal(body, &payload); err != nil {
		return body, "<empty>"
	}

	maskedMetadata, originalUserID := masqueradeMetadata(payload["metadata"])
	payload["metadata"] = maskedMetadata

	maskedBody, err := json.Marshal(payload)
	if err != nil {
		return body, originalUserID
	}

	return maskedBody, originalUserID
}
