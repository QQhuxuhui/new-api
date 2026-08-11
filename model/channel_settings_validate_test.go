package model

import "testing"

// stream_timeout_seconds 服务端范围校验：负数拒绝，0（永不超时）与正数合法
func TestValidateSettingsStreamTimeoutRange(t *testing.T) {
	cases := []struct {
		name    string
		setting string
		wantErr bool
	}{
		{"negative rejected", `{"stream_timeout_seconds":-1}`, true},
		{"zero allowed", `{"stream_timeout_seconds":0}`, false},
		{"positive allowed", `{"stream_timeout_seconds":600}`, false},
		{"max allowed", `{"stream_timeout_seconds":604800}`, false},
		{"over max rejected", `{"stream_timeout_seconds":604801}`, true},
		{"overflow-scale rejected", `{"stream_timeout_seconds":9223372037}`, true},
		{"absent allowed", `{}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setting := tc.setting
			channel := &Channel{Setting: &setting}
			err := channel.ValidateSettings()
			if (err != nil) != tc.wantErr {
				t.Fatalf("ValidateSettings(%s) err=%v, wantErr=%v", tc.setting, err, tc.wantErr)
			}
		})
	}
}
