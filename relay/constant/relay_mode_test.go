package constant

import "testing"

func TestPath2RelayModeImageEditsAliasIsExact(t *testing.T) {
	cases := []struct {
		path string
		want int
	}{
		{"/v1/edits", RelayModeImagesEdits},
		{"/v1/images/edits", RelayModeImagesEdits},
		{"/v1/editsomething", RelayModeUnknown},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			if got := Path2RelayMode(tc.path); got != tc.want {
				t.Fatalf("Path2RelayMode(%q) = %d, want %d", tc.path, got, tc.want)
			}
		})
	}
}
