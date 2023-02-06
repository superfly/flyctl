package redis

import (
	"context"
	"fmt"

	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/iostreams"
)

func AttachDatabase(ctx context.Context, db *RedisAddOn, appName string) (err error) {
	client := client.FromContext(ctx).API()
	io := iostreams.FromContext(ctx)
	s := map[string]string{}
	s["REDIS_URL"] = db.PublicUrl

	_, err = client.SetSecrets(ctx, appName, s)

	if err != nil {
		fmt.Fprintf(io.Out, "\nCould not attach Redis database %s to app %s\n", db.Name, appName)
	} else {
		fmt.Fprintf(io.Out, "\nRedis database %s is set on %s as the REDIS_URL environment variable\n", db.Name, appName)
	}

	return err
}
