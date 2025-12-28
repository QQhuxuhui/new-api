package claude

import (
	"encoding/json"
)

// masqueradeMetadata sets metadata.user_id using a channel-level session pool while
// preserving other metadata fields when possible. It returns the masked
// metadata payload, the original user_id (if present), and the masked user_id.
func masqueradeMetadata(raw json.RawMessage, channelID int, channelHash string) (json.RawMessage, string, string) {
	// Back-compat fallback: if no channel context is available, keep the historical fixed user_id.
	if channelID == 0 && channelHash == "" {
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

		meta["user_id"] = MasqueradeUserID
		masked, err := json.Marshal(meta)
		if err != nil {
			return json.RawMessage(`{"user_id":"` + MasqueradeUserID + `"}`), originalUserID, MasqueradeUserID
		}
		return masked, originalUserID, MasqueradeUserID
	}

	pool := GetSessionPoolManager().GetPool(channelID, channelHash)
	return pool.MasqueradeMetadata(raw)
}

// MasqueradeMetadataInBody updates the top-level metadata field in a Claude
// request body, preserving other top-level fields. It returns the updated body
// and the original user_id (if any) from metadata.
func MasqueradeMetadataInBody(body []byte, channelID int, channelHash string) ([]byte, string, string) {
	payload := make(map[string]json.RawMessage)
	if err := json.Unmarshal(body, &payload); err != nil {
		return body, "<empty>", ""
	}

	maskedMetadata, originalUserID, maskedUserID := masqueradeMetadata(payload["metadata"], channelID, channelHash)
	payload["metadata"] = maskedMetadata

	maskedBody, err := json.Marshal(payload)
	if err != nil {
		return body, originalUserID, maskedUserID
	}

	return maskedBody, originalUserID, maskedUserID
}
