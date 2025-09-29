package appconfig

import (
	"context"

	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/launchdarkly"
)

func DetermineCompression(defaultCompression string, ldClient *launchdarkly.Client, appConfig *Config, ctx context.Context) (compression string, compressionLevel int) {
	compression = defaultCompression
	if ldClient.UseZstdEnabled() {
		compression = "zstd"
	}
	if appConfig.Experimental != nil && appConfig.Experimental.Compression != "" {
		compression = appConfig.Experimental.Compression
	}
	if flag.IsSpecified(ctx, "compression") {
		compression = flag.GetString(ctx, "compression")
	}

	compressionLevel = 7
	compressionLevel = ldClient.GetCompressionStrength()
	if appConfig.Experimental != nil && appConfig.Experimental.CompressionLevel != nil {
		compressionLevel = *appConfig.Experimental.CompressionLevel
	}
	if flag.IsSpecified(ctx, "compression-level") {
		compressionLevel = flag.GetInt(ctx, "compression-level")
	}

	return
}
