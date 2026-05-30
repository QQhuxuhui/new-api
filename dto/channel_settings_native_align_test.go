package dto

import (
	"encoding/json"
	"testing"
)

func TestChannelSettingsNativeAlignRoundTrip(t *testing.T) {
	var s ChannelSettings
	if err := json.Unmarshal([]byte(`{"native_align":true}`), &s); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !s.NativeAlign {
		t.Fatalf("expected NativeAlign=true, got false")
	}

	out, err := json.Marshal(ChannelSettings{NativeAlign: true})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !json.Valid(out) || !contains(string(out), `"native_align":true`) {
		t.Fatalf("marshal missing native_align: %s", out)
	}

	// omitempty: zero value must not emit the key
	out2, _ := json.Marshal(ChannelSettings{})
	if contains(string(out2), "native_align") {
		t.Fatalf("zero value should omit native_align: %s", out2)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
