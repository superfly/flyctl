package redis

import (
	"context"
	"fmt"

	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/iostreams"
)

func AttachDatabase(ctx context.Context, db *gql.AddOn, appName string) (err error) {
	client := flyutil.ClientFromContext(ctx)
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
