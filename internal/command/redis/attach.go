package redis

import (
	"context"
	"fmt"

	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/appsecrets"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/iostreams"
)

func AttachDatabase(ctx context.Context, db *gql.AddOn, appName string) (err error) {
	io := iostreams.FromContext(ctx)
	s := map[string]string{}
	s["REDIS_URL"] = db.PublicUrl

	flapsClient := flapsutil.ClientFromContext(ctx)
	err = appsecrets.Update(ctx, flapsClient, appName, s, nil)
	if err != nil {
		fmt.Fprintf(io.Out, "\nCould not attach Redis database %s to app %s\n", db.Name, appName)
	} else {
		fmt.Fprintf(io.Out, "\nRedis database %s is set on %s as the REDIS_URL environment variable\n", db.Name, appName)
	}

	return err
}
