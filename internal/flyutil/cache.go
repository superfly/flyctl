package flyutil

import (
	"bytes"
	"context"
	"time"

	"github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/cache"
	"gopkg.in/yaml.v3"
)

type CachedClient struct {
	cache.Cache
	Client
}

func convert[T any](in any, err error) (T, error) {
	var out T
	if err != nil {
		return out, err
	}
	b := new(bytes.Buffer)
	err = yaml.NewEncoder(b).Encode(in)
	if err != nil {
		return out, err
	}
	err = yaml.NewDecoder(b).Decode(&out)
	return out, err
}

func (c *CachedClient) GetAppCompact(ctx context.Context, appName string) (*fly.AppCompact, error) {
	return convert[*fly.AppCompact](c.Cache.Fetch("GetAppCompact:"+appName, 1*time.Hour, 1*time.Minute, func() (any, error) {
		return c.Client.GetAppCompact(ctx, appName)
	}))
}

func (c *CachedClient) GetAppBasic(ctx context.Context, appName string) (*fly.AppBasic, error) {
	return convert[*fly.AppBasic](c.Cache.Fetch("GetAppBasic:"+appName, 1*time.Hour, 1*time.Minute, func() (any, error) {
		return c.Client.GetAppBasic(ctx, appName)
	}))
}

func (c *CachedClient) GetOrganizations(ctx context.Context, filters ...fly.OrganizationFilter) ([]fly.Organization, error) {
	if len(filters) > 0 {
		return c.Client.GetOrganizations(ctx, filters...)
	}
	return convert[[]fly.Organization](c.Cache.Fetch("GetOrganizations", 1*time.Hour, 1*time.Minute, func() (any, error) {
		return c.Client.GetOrganizations(ctx)
	}))
}

func FetchCertificate(ctx context.Context, cacheKey string, duration time.Duration, fn func() (*fly.IssuedCertificate, error)) (*fly.IssuedCertificate, error) {
	c := cache.FromContext(ctx)
	return convert[*fly.IssuedCertificate](c.Fetch("FetchCertificate:"+cacheKey, duration, duration, func() (any, error) {
		return fn()
	}))
}

func (c *CachedClient) GetAppNetwork(ctx context.Context, appName string) (*string, error) {
	return convert[*string](c.Cache.Fetch("GetAppNetwork:"+appName, 1*time.Hour, 1*time.Minute, func() (any, error) {
		return c.Client.GetAppNetwork(ctx, appName)
	}))
}
