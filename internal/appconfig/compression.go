package appconfig

import (
	"context"

	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/launchdarkly"
)

func DetermineCompression(ldClient *launchdarkly.Client, appConfig *Config, ctx context.Context) (compression string, compressionLevel int) {
	// Set default values
	compression = "gzip"
	compressionLevel = 7

	// LaunchDarkly provides the base settings
	if ldClient.UseZstdEnabled() {
		compression = "zstd"
	}
	if strength, ok := ldClient.GetCompressionStrength().(float64); ok {
		compressionLevel = int(strength)
	}

	// fly.toml overrides LaunchDarkly
	if appConfig.Experimental != nil {
		if appConfig.Experimental.Compression != "" {
			compression = appConfig.Experimental.Compression
		}
		if appConfig.Experimental.CompressionLevel != nil {
			compressionLevel = *appConfig.Experimental.CompressionLevel
		}
	}

	// CLI flags override everything
	if flag.IsSpecified(ctx, "compression") {
		compression = flag.GetString(ctx, "compression")
	}
	if flag.IsSpecified(ctx, "compression-level") {
		compressionLevel = flag.GetInt(ctx, "compression-level")
	}

	return
}
