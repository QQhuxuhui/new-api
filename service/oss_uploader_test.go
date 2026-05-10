package service

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
)

// 这些测试覆盖 oss_uploader.go 的"配置缺失"分支。
// 真实的 OSS PutObject 调用没法在单测中 mock 阿里云返回,
// 所以这里只覆盖配置缺失的早返路径(用户最常误用)。

func clearOSSConfigForTest() {
	common.OSSAccessKeyId = ""
	common.OSSAccessKeySecret = ""
	common.OSSEndpoint = ""
	common.OSSBucket = ""
}

func TestUploadFileToOSS_RejectsMissingAccessKeyId(t *testing.T) {
	clearOSSConfigForTest()
	common.OSSAccessKeySecret = "x"
	common.OSSEndpoint = "oss-cn-shanghai.aliyuncs.com"
	common.OSSBucket = "x"

	_, err := UploadFileToOSS(strings.NewReader("data"), "posters/poster_xxx.jpg", "image/jpeg")
	if err == nil {
		t.Fatal("expected error when AccessKeyId is empty, got nil")
	}
	if !strings.Contains(err.Error(), "OSS") {
		t.Errorf("error message should mention OSS: %v", err)
	}
}

func TestUploadFileToOSS_RejectsMissingSecret(t *testing.T) {
	clearOSSConfigForTest()
	common.OSSAccessKeyId = "x"
	common.OSSEndpoint = "oss-cn-shanghai.aliyuncs.com"
	common.OSSBucket = "x"

	_, err := UploadFileToOSS(strings.NewReader("data"), "posters/poster_xxx.jpg", "image/jpeg")
	if err == nil {
		t.Fatal("expected error when AccessKeySecret is empty")
	}
}

func TestUploadFileToOSS_RejectsMissingEndpoint(t *testing.T) {
	clearOSSConfigForTest()
	common.OSSAccessKeyId = "x"
	common.OSSAccessKeySecret = "x"
	common.OSSBucket = "x"

	_, err := UploadFileToOSS(strings.NewReader("data"), "posters/poster_xxx.jpg", "image/jpeg")
	if err == nil {
		t.Fatal("expected error when Endpoint is empty")
	}
}

func TestUploadFileToOSS_RejectsMissingBucket(t *testing.T) {
	clearOSSConfigForTest()
	common.OSSAccessKeyId = "x"
	common.OSSAccessKeySecret = "x"
	common.OSSEndpoint = "oss-cn-shanghai.aliyuncs.com"

	_, err := UploadFileToOSS(strings.NewReader("data"), "posters/poster_xxx.jpg", "image/jpeg")
	if err == nil {
		t.Fatal("expected error when Bucket is empty")
	}
}

// BuildPosterPublicURL 是给定 endpoint + bucket + objectKey 构造 public URL 的纯函数,
// 易于测试,且 controller 直接复用。
func TestBuildPosterPublicURL(t *testing.T) {
	got := BuildPosterPublicURL("oss-cn-shanghai.aliyuncs.com", "my-bucket", "posters/poster_abc.jpg")
	want := "https://my-bucket.oss-cn-shanghai.aliyuncs.com/posters/poster_abc.jpg"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

func TestBuildPosterPublicURL_StripsHTTPSPrefixIfPresent(t *testing.T) {
	// 用户在 endpoint 输入框带了 https:// 前缀,应被 strip
	got := BuildPosterPublicURL("https://oss-cn-shanghai.aliyuncs.com", "my-bucket", "posters/x.jpg")
	want := "https://my-bucket.oss-cn-shanghai.aliyuncs.com/posters/x.jpg"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}
