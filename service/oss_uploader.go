package service

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/QuantumNous/new-api/common"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
)

// UploadFileToOSS 把一个文件上传到阿里云 OSS,返回 public URL。
//
// 先校验 4 个 OSS 配置非空,再用 SDK PutObject。
// 调用方:海报上传 controller(controller/poster.go)。
//
// 文件名(objectKey)由调用方决定。海报场景固定用 `posters/poster_<uuid><ext>`,
// 其他场景未来可扩展(如 avatars/...)。
func UploadFileToOSS(reader io.Reader, objectKey, contentType string) (string, error) {
	if common.OSSAccessKeyId == "" {
		return "", errors.New("OSS 未配置:AccessKeyId 为空,请先在后台 Setting 页填写")
	}
	if common.OSSAccessKeySecret == "" {
		return "", errors.New("OSS 未配置:AccessKeySecret 为空,请先在后台 Setting 页填写")
	}
	if common.OSSEndpoint == "" {
		return "", errors.New("OSS 未配置:Endpoint 为空,请先在后台 Setting 页填写")
	}
	if common.OSSBucket == "" {
		return "", errors.New("OSS 未配置:Bucket 为空,请先在后台 Setting 页填写")
	}

	endpoint := normalizeOSSEndpoint(common.OSSEndpoint)
	client, err := oss.New(endpoint, common.OSSAccessKeyId, common.OSSAccessKeySecret)
	if err != nil {
		return "", fmt.Errorf("OSS client 初始化失败: %w", err)
	}
	bucket, err := client.Bucket(common.OSSBucket)
	if err != nil {
		return "", fmt.Errorf("OSS bucket 获取失败: %w", err)
	}

	opts := []oss.Option{}
	if contentType != "" {
		opts = append(opts, oss.ContentType(contentType))
	}

	if err := bucket.PutObject(objectKey, reader, opts...); err != nil {
		return "", fmt.Errorf("OSS 上传失败: %w", err)
	}

	return BuildPosterPublicURL(common.OSSEndpoint, common.OSSBucket, objectKey), nil
}

// BuildPosterPublicURL 根据 endpoint + bucket + objectKey 构造 OSS 公开访问 URL。
// 形式:`https://<bucket>.<endpoint>/<objectKey>`
//
// 抽出独立函数便于测试和 controller 复用。
func BuildPosterPublicURL(endpoint, bucket, objectKey string) string {
	host := normalizeOSSEndpoint(endpoint)
	return fmt.Sprintf("https://%s.%s/%s", bucket, host, objectKey)
}

// normalizeOSSEndpoint 去掉用户配置时可能带的 `http(s)://` 前缀,只保留 host。
func normalizeOSSEndpoint(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "http://")
	return s
}
