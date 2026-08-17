package model

import (
	"testing"
)

func upscaleChannel(t *testing.T, setting string) *Channel {
	t.Helper()
	ch := &Channel{}
	ch.Setting = &setting
	return ch
}

func TestChannelSatisfiesFilterWithUpscale(t *testing.T) {
	setting := `{"image_sizes":{"allowed":["1K"],"upscale":{"from":"1K","to":"4K"}}}`
	ch := upscaleChannel(t, setting)

	f := &ChannelSelectFilter{ImageSizeTier: "4K", UpscaleEligible: true}
	if !channelSatisfiesFilter(ch, f) {
		t.Fatal("eligible 时 4K 应派生可达")
	}
	if f.imageSizeViaUpscale == 0 {
		t.Fatal("经派生通过应打 viaUpscale 计数")
	}

	f2 := &ChannelSelectFilter{ImageSizeTier: "4K", UpscaleEligible: false}
	if channelSatisfiesFilter(ch, f2) {
		t.Fatal("不具资格时 4K 应被拒")
	}

	f3 := &ChannelSelectFilter{ImageSizeTier: "1K", UpscaleEligible: false}
	if !channelSatisfiesFilter(ch, f3) {
		t.Fatal("原生 1K 不受资格影响")
	}
	if f3.imageSizeViaUpscale != 0 {
		t.Fatal("原生通过不应打 viaUpscale 计数")
	}
}
