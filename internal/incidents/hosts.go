package incidents

import (
	"context"
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

	statusCh := make(chan []fly.HostIssue, 1)
	logger.Debug("started querying for host issues")
	statusCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
	go func() {
		defer cancel()
		defer close(statusCh)
		response, err := GetAppHostIssuesRequest(statusCtx, appName)
		if err != nil {
			logger.Debugf("failed querying for host issues: %v", err)
		}
		statusCh <- response
	}()

	task.FromContext(ctx).RunFinalizer(func(parent context.Context) {
		cancel()
		select {
		case hostIssues := <-statusCh:
			logger.Debugf("querying for host issues resulted to %v", hostIssues)
			hostIssuesCount := len(hostIssues)
			if hostIssuesCount > 0 {
				fmt.Fprintln(io.ErrOut, colorize.WarningIcon(),
					colorize.Yellow("WARNING: There are active host issues affecting your app. Please check `fly incidents hosts list` or visit your app in https://fly.io/dashboard\n"),
				)
				break
			}
		}
	})
}
