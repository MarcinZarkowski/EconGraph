package s3

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/parquet-go/parquet-go"
)

const DefaultBucketName = "stonkydb-assets"
const DefaultFullGraphKey = "graphs/latest/full_graph.json.gz"

type Client struct {
	client *s3.Client
	Bucket string
}

func LoadEnvironment(key string) (string, bool) {
	res := os.Getenv(key)
	if res == "" {
		return "", false
	}
	return res, true
}

func NewClient(ctx context.Context) (*Client, error) {
	bucket := os.Getenv("AWS_S3_BUCKET")
	if bucket == "" {
		bucket = DefaultBucketName
	}

	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "us-east-2"
	}

	var optFns []func(*config.LoadOptions) error
	optFns = append(optFns, config.WithRegion(region))

	accessKey := os.Getenv("AWS_ACCESS_KEY")
	if accessKey == "" {
		accessKey = os.Getenv("AWS_ACCESS_KEY_ID")
	}
	secretKey := os.Getenv("AWS_SECRET_ACCESS_KEY")

	if accessKey != "" && secretKey != "" {
		optFns = append(optFns, config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(accessKey, secretKey, ""),
		))
	}

	cfg, err := config.LoadDefaultConfig(ctx, optFns...)
	if err != nil {
		return nil, fmt.Errorf("failed to load aws config: %w", err)
	}

	var s3OptFns []func(*s3.Options)
	if os.Getenv("AWS_ENDPOINT_URL") != "" {
		s3OptFns = append(s3OptFns, func(o *s3.Options) {
			o.UsePathStyle = true
		})
	}

	return &Client{
		client: s3.NewFromConfig(cfg, s3OptFns...),
		Bucket: bucket,
	}, nil
}

type ProtoFileReader[T any] struct {
	s3  *Client
	key string
}

func NewProtoFileReader[T any](s3 *Client, key string) *ProtoFileReader[T] {
	return &ProtoFileReader[T]{
		s3:  s3,
		key: key,
	}
}

func (p *ProtoFileReader[T]) Read(ctx context.Context) func(yield func(*T, error) bool) {
	return func(yield func(*T, error) bool) {
		resp, err := p.s3.client.GetObject(ctx, &s3.GetObjectInput{
			Bucket: &p.s3.Bucket,
			Key:    &p.key,
		})
		if err != nil {
			yield(nil, err)
			return
		}
		defer resp.Body.Close()

		tmp, err := os.CreateTemp("", "parquet-*")
		if err != nil {
			yield(nil, fmt.Errorf("create temp file: %w", err))
			return
		}
		defer func() {
			tmp.Close()
			os.Remove(tmp.Name())
		}()

		if _, err := io.Copy(tmp, resp.Body); err != nil {
			yield(nil, fmt.Errorf("copy stream to temp: %w", err))
			return
		}

		reader := parquet.NewGenericReader[T](tmp)
		defer reader.Close()

		batch := make([]T, 1024)
		for {
			n, err := reader.Read(batch)
			for i := 0; i < n; i++ {
				if !yield(&batch[i], nil) {
					return
				}
			}

			if errors.Is(err, io.EOF) {
				return
			}
			if err != nil {
				yield(nil, err)
				return
			}
		}
	}
}
