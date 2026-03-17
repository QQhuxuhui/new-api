package openai

import (
	"encoding/json"
	"strings"
)

func normalizeResponsesInputIdentifiers(input json.RawMessage) (json.RawMessage, error) {
	if len(input) == 0 {
		return input, nil
	}

	var items []map[string]any
	if err := json.Unmarshal(input, &items); err != nil {
		// Non-array inputs are left untouched.
		return input, nil
	}

	changed := false
	for _, item := range items {
		if item == nil {
			continue
		}
		switch stringField(item["type"]) {
		case "function_call":
			if normalizeFunctionCallItem(item) {
				changed = true
			}
		case "function_call_output":
			if normalizeFunctionCallOutputItem(item) {
				changed = true
			}
		}
	}

	if !changed {
		return input, nil
	}
	return json.Marshal(items)
}

func normalizeFunctionCallItem(item map[string]any) bool {
	originalCallID := stringField(item["call_id"])
	originalItemID := stringField(item["id"])

	callID := originalCallID
	itemID := originalItemID

	if splitCallID, splitItemID, ok := splitCompoundToolCallID(callID); ok {
		callID = splitCallID
		if itemID == "" {
			itemID = splitItemID
		}
	}

	if splitCallID, splitItemID, ok := splitCompoundToolCallID(itemID); ok {
		if callID == "" {
			callID = splitCallID
		}
		itemID = splitItemID
	}

	if callID == "" && looksLikeResponsesCallID(itemID) {
		callID = itemID
		itemID = ""
	}

	if itemID != "" && !looksLikeResponsesFunctionCallItemID(itemID) {
		itemID = ""
	}

	changed := callID != originalCallID || itemID != originalItemID
	if !changed {
		return false
	}

	if callID != "" {
		item["call_id"] = callID
	} else {
		delete(item, "call_id")
	}

	if itemID != "" {
		item["id"] = itemID
	} else {
		delete(item, "id")
	}
	return true
}

func normalizeFunctionCallOutputItem(item map[string]any) bool {
	originalCallID := stringField(item["call_id"])
	originalItemID := stringField(item["id"])

	callID := originalCallID
	if splitCallID, _, ok := splitCompoundToolCallID(callID); ok {
		callID = splitCallID
	}
	if callID == "" && looksLikeResponsesCallID(originalItemID) {
		callID = originalItemID
	}

	changed := callID != originalCallID
	if !changed {
		return false
	}

	if callID != "" {
		item["call_id"] = callID
	} else {
		delete(item, "call_id")
	}

	if originalCallID == "" && originalItemID != "" && looksLikeResponsesCallID(originalItemID) {
		delete(item, "id")
	}

	return true
}

func splitCompoundToolCallID(raw string) (callID string, itemID string, ok bool) {
	parts := strings.SplitN(raw, "|", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	callID = strings.TrimSpace(parts[0])
	itemID = strings.TrimSpace(parts[1])
	if callID == "" || itemID == "" {
		return "", "", false
	}
	return callID, itemID, true
}

func looksLikeResponsesFunctionCallItemID(value string) bool {
	return strings.HasPrefix(strings.TrimSpace(value), "fc")
}

func looksLikeResponsesCallID(value string) bool {
	return strings.HasPrefix(strings.TrimSpace(value), "call")
}

func stringField(value any) string {
	str, _ := value.(string)
	return strings.TrimSpace(str)
}
