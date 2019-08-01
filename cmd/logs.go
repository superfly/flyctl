package cmd

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"time"

	"github.com/logrusorgru/aurora"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/terminal"
)

func newAppLogsCommand() *cobra.Command {
	logs := &appLogsCommand{}

	cmd := &cobra.Command{
		Use:   "logs",
		Short: "view app logs",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return logs.Init()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return logs.Run(args)
		},
	}

	f := cmd.Flags()
	f.StringVarP(&logs.appName, "app", "a", "", `app to run command against`)

	return cmd
}

type appLogsCommand struct {
	appName   string
	nextToken string
	client    *http.Client
}

func (cmd *appLogsCommand) Init() error {
	if cmd.appName == "" {
		cmd.appName = flyctl.CurrentAppName()
	}
	if cmd.appName == "" {
		return fmt.Errorf("no app specified")
	}

	return nil
}

type respData struct {
	Data []struct {
		Id         string
		Attributes logEntry
	}
	Meta struct {
		NextToken string `json:"next_token"`
	}
}

type logEntry struct {
	Timestamp string
	Message   string
	Level     string
	Meta      struct {
		Instance string
		Region   string
	}
}

func (cmd *appLogsCommand) Run(args []string) error {
	cmd.client = &http.Client{}

	errorCount := 0
	emptyCount := 0

	for {
		data, err := cmd.fetchLogs()

		if err != nil {
			if api.IsNotAuthenticatedError(err) {
				return err
			} else if api.IsNotFoundError(err) {
				return err
			} else {
				errorCount++
				if errorCount > 3 {
					return err
				}
				sleep(errorCount)
			}
		}
		errorCount = 0

		if len(data.Data) == 0 {
			emptyCount++
			sleep(emptyCount)
		} else {
			emptyCount = 0
			printLogs(data)

			if data.Meta.NextToken != "" {
				cmd.nextToken = data.Meta.NextToken
			}
		}
	}

	return nil
}

var maxBackoff float64 = 5000

func sleep(backoffCount int) {
	sleepTime := math.Pow(float64(backoffCount), 2) * 250
	if sleepTime > maxBackoff {
		sleepTime = maxBackoff
	}
	terminal.Debug("backoff ms:", sleepTime)
	time.Sleep(time.Duration(sleepTime) * time.Millisecond)
}

func printLogs(entries respData) {
	for _, entry := range entries.Data {
		printLogEntry(entry.Attributes)
	}
}

func printLogEntry(entry logEntry) {
	fmt.Printf(
		"%s %s %s [%s] %s\n",
		aurora.Faint(entry.Timestamp),
		entry.Meta.Instance,
		aurora.Green(entry.Meta.Region),
		aurora.Colorize(entry.Level, levelColor(entry.Level)),
		entry.Message,
	)
}

func levelColor(level string) aurora.Color {
	switch level {
	case "debug":
		return aurora.CyanFg
	case "info":
		return aurora.BlueFg
	case "warning":
		return aurora.MagentaFg
	}
	return aurora.RedFg
}

func (cmd *appLogsCommand) fetchLogs() (respData, error) {
	data := url.Values{}
	data.Set("next_token", cmd.nextToken)

	url := fmt.Sprintf("%s/api/v1/apps/%s/logs?%s", viper.GetString(flyctl.ConfigAPIBaseURL), cmd.appName, data.Encode())

	req, err := http.NewRequest("GET", url, nil)
	accessToken := viper.GetString(flyctl.ConfigAPIAccessToken)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))

	var result respData

	resp, err := cmd.client.Do(req)
	if err != nil {
		return result, err
	}

	if resp.StatusCode != 200 {
		return result, api.ErrorFromResp(resp)
	}

	defer resp.Body.Close()

	json.NewDecoder(resp.Body).Decode(&result)

	return result, nil
}
