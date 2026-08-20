package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// upscaleObjectStore 抽掉具体存储：生产是 S3 兼容实现（R2/OSS 同一套），
// 测试用内存 mock。worker 侧零凭据——它只拿到 new-api 预签好的 GET/PUT URL。
type upscaleObjectStore interface {
	PutObject(ctx context.Context, key string, data []byte, contentType string) error
	GetObject(ctx context.Context, key string) ([]byte, error)
	DeleteObject(ctx context.Context, key string) error
	PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error)
	PresignPut(ctx context.Context, key string, contentType string, ttl time.Duration) (string, error)
}

type s3UpscaleStore struct {
	client  *s3.Client
	presign *s3.PresignClient
	bucket  string
}

func newS3UpscaleStore(cfg *ImageUpscaleConfig) (upscaleObjectStore, error) {
	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion(cfg.S3Region),
		awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.S3AccessKey, cfg.S3SecretKey, "")),
	)
	if err != nil {
		return nil, err
	}
	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(cfg.S3Endpoint)
		o.UsePathStyle = true // R2 与 OSS 的 S3 兼容层都接受 path-style，统一之
	})
	return &s3UpscaleStore{client: client, presign: s3.NewPresignClient(client), bucket: cfg.S3Bucket}, nil
}

func (s *s3UpscaleStore) PutObject(ctx context.Context, key string, data []byte, contentType string) error {
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket), Key: aws.String(key),
		Body: bytes.NewReader(data), ContentType: aws.String(contentType),
	})
	return err
}

// maxUpscaleObjectBytes 是从对象存储取回结果的字节上限。合法上限是 4096²
// 高熵 PNG（compress_level=1 实测 ~44MB），128MB 留足余量；再大只可能是
// 异常写入，不该无界读进内存。
const maxUpscaleObjectBytes = 128 << 20

// readAllLimited 读到上限即报错（区别于静默截断——截断的图会通过部分解码）。
func readAllLimited(r io.Reader, max int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > max {
		return nil, fmt.Errorf("object exceeds %dMB cap", max>>20)
	}
	return data, nil
}

func (s *s3UpscaleStore) GetObject(ctx context.Context, key string) ([]byte, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)})
	if err != nil {
		return nil, err
	}
	defer out.Body.Close()
	return readAllLimited(out.Body, maxUpscaleObjectBytes)
}

func (s *s3UpscaleStore) DeleteObject(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	return err
}

func (s *s3UpscaleStore) PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error) {
	req, err := s.presign.PresignGetObject(ctx,
		&s3.GetObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)},
		s3.WithPresignExpires(ttl))
	if err != nil {
		return "", err
	}
	return req.URL, nil
}

func (s *s3UpscaleStore) PresignPut(ctx context.Context, key string, contentType string, ttl time.Duration) (string, error) {
	req, err := s.presign.PresignPutObject(ctx,
		&s3.PutObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key), ContentType: aws.String(contentType)},
		s3.WithPresignExpires(ttl))
	if err != nil {
		return "", err
	}
	return req.URL, nil
}
