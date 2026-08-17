package service

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"

	"github.com/tidwall/gjson"
)

func fakeUp(out []byte, err error) imageUpscaleFunc {
	return func(_ context.Context, _ []byte, _, _ int) ([]byte, error) { return out, err }
}

func TestRewriteImageResponseWithUpscale(t *testing.T) {
	src := pngBytes(t, 32, 32)
	big := pngBytes(t, 128, 128)
	body := []byte(`{"created":1,"size":"32x32","data":[{"b64_json":"` +
		base64.StdEncoding.EncodeToString(src) + `","size":"32x32"}],"usage":{"total_tokens":10}}`)

	out, err := RewriteImageResponseWithUpscale(context.Background(), body, 128, 128, fakeUp(big, nil))
	if err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	gotB64 := gjson.GetBytes(out, "data.0.b64_json").String()
	decoded, _ := base64.StdEncoding.DecodeString(gotB64)
	if len(decoded) != len(big) {
		t.Fatal("b64_json 应替换为放大后的图")
	}
	if gjson.GetBytes(out, "size").String() != "128x128" {
		t.Fatal("根级 size 声明必须改写（sub2api 兜底路径读它）")
	}
	if gjson.GetBytes(out, "data.0.size").String() != "128x128" {
		t.Fatal("data 项级 size 声明必须改写")
	}
	if gjson.GetBytes(out, "usage.total_tokens").Int() != 10 {
		t.Fatal("usage 必须原样保留")
	}
}

func TestRewriteNoSizeFieldsStaysAbsent(t *testing.T) {
	src := pngBytes(t, 32, 32)
	body := []byte(`{"data":[{"b64_json":"` + base64.StdEncoding.EncodeToString(src) + `"}]}`)
	out, err := RewriteImageResponseWithUpscale(context.Background(), body, 128, 128, fakeUp(pngBytes(t, 128, 128), nil))
	if err != nil {
		t.Fatal(err)
	}
	if gjson.GetBytes(out, "size").Exists() {
		t.Fatal("原本没有的 size 字段不应凭空出现")
	}
}

func TestRewriteFailurePropagates(t *testing.T) {
	src := pngBytes(t, 32, 32)
	body := []byte(`{"data":[{"b64_json":"` + base64.StdEncoding.EncodeToString(src) + `"}]}`)
	if _, err := RewriteImageResponseWithUpscale(context.Background(), body, 128, 128, fakeUp(nil, errors.New("gpu down"))); err == nil {
		t.Fatal("超分失败必须报错（由调用方降级）")
	}
	if _, err := RewriteImageResponseWithUpscale(context.Background(), []byte(`{"data":[]}`), 128, 128, fakeUp(nil, nil)); err == nil {
		t.Fatal("空 data 必须报错")
	}
	if _, err := RewriteImageResponseWithUpscale(context.Background(), []byte(`{"data":[{"url":"http://x"}]}`), 128, 128, fakeUp(nil, nil)); err == nil {
		t.Fatal("无 b64_json（url 响应）必须报错——资格谓词已排除该形状，走到这说明上游违约")
	}
}
