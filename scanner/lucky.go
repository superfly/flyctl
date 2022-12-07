package scanner

import (
	"context"

	"github.com/superfly/flyctl/helpers"
)

func configureLucky(ctx context.Context, sourceDir string) (*SourceInfo, error) {
	if !checksPass(sourceDir, dirContains("shard.yml", "lucky")) {
		return nil, nil
	}

	s := &SourceInfo{
		Family:     "Lucky",
		Files:      templates("templates/lucky"),
		Port:       8080,
		ReleaseCmd: "lucky db.migrate",
		Env: map[string]string{
			"PORT":       "8080",
			"LUCKY_ENV":  "production",
			"APP_DOMAIN": "APP_FQDN",
		},
		Secrets: []Secret{
			{
				Key:  "SECRET_KEY_BASE",
				Help: "Lucky needs a random, secret key. Use the random default we've generated, or generate your own.",
				Generate: func() (string, error) {
					return helpers.RandString(64)
				},
			},
			{
				Key:   "SEND_GRID_KEY",
				Help:  "Lucky needs a SendGrid API key. For now, we're setting this to unused. You can generate one at https://docs.sendgrid.com/for-developers/sending-email/api-getting-started",
				Value: "unused",
			},
		},
		Statics: []Static{
			{
				GuestPath: "/app/public",
				UrlPrefix: "/",
			},
		},
	}

	return s, nil
}
