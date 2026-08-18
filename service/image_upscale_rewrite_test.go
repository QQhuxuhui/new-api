package service

import (
	"context"
	"encoding/base64"
	"errors"
	"strconv"
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

func TestNormalizeImageResponseSize(t *testing.T) {
	src := pngBytes(t, 1254, 1254)
	body := []byte(`{"size":"1024x1024","data":[{"b64_json":"` + base64.StdEncoding.EncodeToString(src) + `"}]}`)

	// 尺寸不一致 → 走重采样并改写
	out, changed, err := NormalizeImageResponseSize(context.Background(), body, 1024, 1024, fakeUp(pngBytes(t, 1024, 1024), nil))
	if err != nil || !changed {
		t.Fatalf("mismatch 应触发规整: changed=%v err=%v", changed, err)
	}
	if gjson.GetBytes(out, "size").String() != "1024x1024" {
		t.Fatal("声明 size 应为规整后尺寸")
	}

	// 尺寸一致 → 原样返回不动
	match := []byte(`{"data":[{"b64_json":"` + base64.StdEncoding.EncodeToString(pngBytes(t, 1024, 1024)) + `"}]}`)
	out2, changed2, err := NormalizeImageResponseSize(context.Background(), match, 1024, 1024, fakeUp(nil, errors.New("不应被调用")))
	if err != nil || changed2 {
		t.Fatalf("一致时应 no-op: changed=%v err=%v", changed2, err)
	}
	if string(out2) != string(match) {
		t.Fatal("一致时 body 应原样")
	}

	// 解码失败 → 报错（调用方降级）
	bad := []byte(`{"data":[{"b64_json":"` + base64.StdEncoding.EncodeToString([]byte("not-an-image")) + `"}]}`)
	if _, _, err := NormalizeImageResponseSize(context.Background(), bad, 1024, 1024, fakeUp(nil, nil)); err == nil {
		t.Fatal("非图片字节必须报错")
	}
}

// TestNormalizeImageResponseSizeDimensionCap 锁定规整链路的单边像素上限。
//
// 规整的目标尺寸直接取用户请求的任意 WxH，源图尺寸由上游决定，两头都没有超分
// 那样的 Upscale.To 封顶。worker 的 16GB 档前提是"源图 ≤2048 + tiling"，而
// Real-ESRGAN x4 会先把源图放大 4 倍再 Lanczos 精确缩放——4096 的源图中间态就是
// 16384²。所以任一边超过上限就必须直接放弃规整（原样返回、changed=false、
// 不报错），绝不能把这种任务扔给 worker。
func TestNormalizeImageResponseSizeDimensionCap(t *testing.T) {
	const cap = normalizeMaxDimension

	cases := []struct {
		name             string
		srcW, srcH       int
		targetW, targetH int
	}{
		{"源图宽超上限", cap + 1, 512, 1024, 1024},
		{"源图高超上限", 512, cap + 1, 1024, 1024},
		{"目标宽超上限", 512, 512, cap + 1, 1024},
		{"目标高超上限", 512, 512, 1024, cap + 1},
		{"源图与目标同时超上限", cap + 1, cap + 1, cap + 2, cap + 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := pngBytes(t, tc.srcW, tc.srcH)
			body := []byte(`{"data":[{"b64_json":"` + base64.StdEncoding.EncodeToString(src) + `"}]}`)
			called := false
			up := func(_ context.Context, _ []byte, _, _ int) ([]byte, error) {
				called = true
				return nil, errors.New("worker 不该被调用")
			}
			out, changed, err := NormalizeImageResponseSize(context.Background(), body, tc.targetW, tc.targetH, up)
			if err != nil {
				t.Fatalf("超上限属'不适用'而非'失败'，不应报错: %v", err)
			}
			if changed {
				t.Fatal("超上限时必须返回 changed=false")
			}
			if called {
				t.Fatal("超上限时绝不能调用 worker")
			}
			if string(out) != string(body) {
				t.Fatal("超上限时必须原样返回上游 body")
			}
		})
	}
}

// 边界值本身（恰好等于上限）仍在能力范围内，必须正常规整。
func TestNormalizeImageResponseSizeAtCapStillNormalizes(t *testing.T) {
	const cap = normalizeMaxDimension
	src := pngBytes(t, cap, cap)
	body := []byte(`{"size":"1x1","data":[{"b64_json":"` + base64.StdEncoding.EncodeToString(src) + `"}]}`)
	want := pngBytes(t, cap, 1024)
	out, changed, err := NormalizeImageResponseSize(context.Background(), body, cap, 1024, fakeUp(want, nil))
	if err != nil {
		t.Fatalf("恰好等于上限应正常规整: %v", err)
	}
	if !changed {
		t.Fatal("尺寸不一致且未超上限，应 changed=true")
	}
	wantSize := strconv.Itoa(cap) + "x1024"
	if got := gjson.GetBytes(out, "size").String(); got != wantSize {
		t.Fatalf("声明 size 未改写: got %s want %s", got, wantSize)
	}
}
