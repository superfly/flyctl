package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"sync"

	"github.com/superfly/flyctl/cmdctx"

	"github.com/dustin/go-humanize"
	"github.com/logrusorgru/aurora"
	"github.com/superfly/flyctl/docstrings"

	"github.com/spf13/cobra"
)

func newCurlCommand() *Command {
	curlStrings := docstrings.Get("curl")
	cmd := BuildCommandKS(nil, runCurl, curlStrings, os.Stdout, requireSession)
	cmd.Args = cobra.ExactArgs(1)
	cmd.Hidden = true
	return cmd
}

// TimingResponse - Results from timing a curl operations
type TimingResponse struct {
	Err               error
	HTTPCode          int     `json:"http_code"`
	SpeedDownload     int     `json:"speed_download"`
	TimeTotal         float64 `json:"time_total"`
	TimeNameLookup    float64 `json:"time_namelookup"`
	TimeConnect       float64 `json:"time_connect"`
	TimePreTransfer   float64 `json:"time_pretransfer"`
	TimeAppConnect    float64 `json:"time_appconnect"`
	TimeStartTransfer float64 `json:"time_starttransfer"`
	HTTPVersion       string  `json:"http_version"`
	RemoteIP          string  `json:"remote_ip"`
	Scheme            string  `json:"scheme"`
	Region            string  `json:"region"`
}

// TimingRequest - Request to time a curl operation
type TimingRequest struct {
	URL    string `json:"url"`
	Region string `json:"region"`
}

func runCurl(ctx *cmdctx.CmdContext) error {
	regions, _, err := ctx.Client.API().PlatformRegions()
	if err != nil {
		panic(err)
	}

	url := ctx.Args[0]

	results := make(chan TimingResponse, len(regions))

	client := &http.Client{}

	var wg sync.WaitGroup
	wg.Add(len(regions))

	for _, region := range regions {
		region := region
		go func() {
			timingResp := TimingResponse{
				Region: region.Code,
			}

			body, err := json.Marshal(TimingRequest{URL: url, Region: region.Code})
			if err != nil {
				panic(err)
			}
			req, err := http.NewRequest("POST", "https://curl.fly.dev/timings", bytes.NewBuffer(body))
			if err != nil {
				panic(err)
			}
			req.Header.Add("Authorization", "1q2w3e4r")
			req.Header.Add("Content-Type", "application/json")

			resp, err := client.Do(req)
			if err != nil {
				timingResp.Err = err
			} else {
				defer resp.Body.Close()

				if resp.StatusCode != 200 {
					data, err := ioutil.ReadAll(resp.Body)
					if err != nil {
						timingResp.Err = err
					} else {
						err = errors.New(string(data))
						timingResp.Err = err
					}
				} else {
					err = json.NewDecoder(resp.Body).Decode(&timingResp)
					if err != nil {
						timingResp.Err = err
					}
				}
			}

			results <- timingResp

			wg.Done()
		}()
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	var timingRowFormat = "%s\t%s\t%s\t%s\t%s\t%s\t%s\n"
	var timingRowErrorFormat = "%s\t%s\n"

	failures := []TimingResponse{}

	fmt.Printf(timingRowFormat, "Region", "Status", "DNS", "Connect", "TLS", "TTFB", "Total")
	for result := range results {
		if result.Err != nil {
			failures = append(failures, result)
			continue
		}

		fmt.Printf(timingRowFormat,
			result.Region,
			formatHTTPStatus(result.HTTPCode),
			formatDNS(result),
			formatConnect(result),
			formatTLS(result),
			formatTTFB(result),
			formatTotal(result),
		)
	}

	if len(failures) > 0 {
		fmt.Println("\nFailures:")
		for _, result := range failures {
			fmt.Printf(timingRowErrorFormat, result.Region, result.Err)
		}
	}

	return nil
}

func formatHTTPStatus(status int) interface{} {
	text := strconv.Itoa(status)
	return colorize(text, float64(status), 299, 399)
}

func formatDNS(result TimingResponse) interface{} {
	return humanize.FtoaWithDigits(result.TimeNameLookup*1000, 1) + "ms"
}

func formatConnect(result TimingResponse) interface{} {
	timing := result.TimeConnect * 1000
	text := humanize.FtoaWithDigits(timing, 1) + "ms"
	return colorize(text, timing, 200, 500)
}

func formatTLS(result TimingResponse) interface{} {
	return humanize.FtoaWithDigits((result.TimeAppConnect+result.TimePreTransfer)*1000, 1) + "ms"
}

func formatTTFB(result TimingResponse) interface{} {
	timing := result.TimeStartTransfer * 1000
	text := humanize.FtoaWithDigits(timing, 1) + "ms"
	return colorize(text, timing, 400, 1000)
}

func formatTotal(result TimingResponse) interface{} {
	timing := result.TimeTotal * 1000
	return humanize.FtoaWithDigits(timing, 1) + "ms"
}

func colorize(text string, val float64, greenCutoff float64, yellowCutoff float64) interface{} {
	var color aurora.Color
	switch {
	case val <= greenCutoff:
		color = aurora.GreenFg
	case val <= yellowCutoff:
		color = aurora.YellowFg
	default:
		color = aurora.RedFg
	}

	return aurora.Colorize(text, color)
}
