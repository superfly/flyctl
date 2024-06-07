package incidents

import (
	"context"
	"errors"
	"fmt"
	"time"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/flyutil"

	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/logger"
	"github.com/superfly/flyctl/internal/task"
	"github.com/superfly/flyctl/iostreams"
)

func GetAppHostIssuesRequest(ctx context.Context, appName string) ([]fly.HostIssue, error) {
	client := flyutil.ClientFromContext(ctx)

	appHostIssues, err := client.GetAppHostIssues(ctx, appName)
	if err != nil {
		return nil, err
	}

	return appHostIssues, nil
}

func QueryHostIssues(ctx context.Context) {

	logger := logger.FromContext(ctx)
	io := iostreams.FromContext(ctx)
	colorize := io.ColorScheme()
	appName := appconfig.NameFromContext(ctx)

	if appName == "" {
		return
	}

	task.FromContext(ctx).RunFinalizer(func(parent context.Context) {
		logger.Debug("started querying for host issues")

		ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), 3*time.Second)
		defer cancel()

		switch hostIssues, err := GetAppHostIssuesRequest(ctx, appName); {
		case err == nil:
			if hostIssues == nil {
				break
			}

			logger.Debugf("querying for host issues resulted to %v", hostIssues)
			hostIssuesCount := len(hostIssues)
			if hostIssuesCount > 0 {
				fmt.Fprintln(io.ErrOut, colorize.WarningIcon(),
					colorize.Yellow("WARNING: There are active host issues affecting your app. Please check `fly incidents hosts list` or visit your app in https://fly.io/dashboard\n"),
				)
				break
			}
		case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			logger.Debugf("failed querying for host issues. Context cancelled or deadline exceeded: %v", err)
		default:
			logger.Debugf("failed querying for host issues incidents: %v", err)
		}
	})
}
