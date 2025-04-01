package statics

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sync"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/superfly/fly-go"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/tokenizer"
)

func spawnWorkers(ctx context.Context, n int, f func(context.Context) error) func() error {
	ctx, cancel := context.WithCancel(ctx)

	workerErr := make(chan error, 1)

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := f(ctx); err != nil {
				cancel()
				select {
				case workerErr <- err:
				default:
				}
			}
		}()
	}

	return func() error {

		defer cancel()
		wg.Wait()

		// Check if any of the workers failed.
		select {
		case err := <-workerErr:
			return err
		default:
			return nil
		}
	}
}

func getPushToken(ctx context.Context, org *fly.Organization) (string, error) {
	client := flyutil.ClientFromContext(ctx)

	resp, err := gql.CreateLimitedAccessToken(
		ctx,
		client.GenqClient(),
		"Flyctl statics push token",
		org.ID,
		"deploy_organization",
		&gql.LimitedAccessTokenOptions{},
		"1h",
	)
	if err != nil {
		return "", err
	}
	return resp.CreateLimitedAccessToken.LimitedAccessToken.TokenHeader, nil
}

func s3ClientWithAuth(ctx context.Context, auth string, org *fly.Organization) (*s3.Client, error) {

	s3Config, err := config.LoadDefaultConfig(ctx,
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("tokenizer-access-key", "tokenizer-secret-key", "")),
		config.WithRegion("auto"),
	)
	if err != nil {
		return nil, err
	}
	s3Config.BaseEndpoint = fly.Pointer(tigrisUrl)

	parsedProxyUrl, err := url.Parse(tokenizerUrl)
	if err != nil {
		// Should be impossible, this is not runtime-controlled and issues would be caught before release.
		return nil, fmt.Errorf("could not parse tokenizer URL: %w", err)
	}
	s3HttpTransport := http.DefaultTransport.(*http.Transport).Clone()
	s3HttpTransport.Proxy = http.ProxyURL(parsedProxyUrl)

	userAuthHeader, err := getPushToken(ctx, org)
	if err != nil {
		return nil, err
	}

	s3HttpClient, err := tokenizer.Client(tokenizerUrl, tokenizer.WithAuth(userAuthHeader), tokenizer.WithSecret(auth, map[string]string{}))
	if err != nil {
		return nil, err
	}

	s3Config.HTTPClient = s3HttpClient

	return s3.NewFromConfig(s3Config), nil
}
