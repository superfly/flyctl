// Package curl implements the curl command chain.
package curl

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/logrusorgru/aurora"
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/flag"
	"github.com/superfly/flyctl/internal/cli/internal/render"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/pkg/iostreams"
)

// New initializes and returns a new apps Command.
func New() (cmd *cobra.Command) {
	const (
		short = "Run a performance test against a URL"
		long  = short + "\n"
	)

	cmd = command.New("curl <URL>", short, long, run,
		command.RequireSession,
	)

	cmd.Args = cobra.ExactArgs(1)

	return
}

func run(ctx context.Context) error {
	url, err := url.Parse(flag.FirstArg(ctx))
	if err != nil {
		return fmt.Errorf("invalid URL specified: %w", err)
	}

	regionCodes, err := fetchRegionCodes(ctx)
	if err != nil {
		return err
	}

	rws, err := prepareRequestWrappers(ctx, url, regionCodes)
	if err != nil {
		return err
	}

	timings := gatherTimings(ctx, rws)
	if err := ctx.Err(); err != nil {
		return err
	}

	out := iostreams.FromContext(ctx).Out
	renderTimings(out, timings)

	return nil
}

func fetchRegionCodes(ctx context.Context) (codes []string, err error) {
	client := client.FromContext(ctx).API()

	var regions []api.Region
	if regions, _, err = client.PlatformRegions(ctx); err != nil {
		err = fmt.Errorf("failed retrieving regions: %w", err)

		return
	} else if len(regions) == 0 {
		err = errors.New("no regions could be retrieved")

		return
	}

	for _, region := range regions {
		codes = append(codes, region.Code)
	}
	sort.Strings(codes)

	return
}

func prepareRequestWrappers(ctx context.Context, url *url.URL, regionCodes []string) (rws []*requestWrapper, err error) {
	for _, region := range regionCodes {
		var rw *requestWrapper
		if rw, err = wrapRequestForRegion(ctx, region, url); err != nil {
			err = fmt.Errorf("failed preparing request for %s: %w", region, err)

			break
		}

		rws = append(rws, rw)
	}

	return
}

func gatherTimings(ctx context.Context, rws []*requestWrapper) (timings []*timing) {
	var wg sync.WaitGroup
	wg.Add(len(rws))

	c := make(chan *timing, len(rws))

	for i := range rws {
		go func(rw *requestWrapper) {
			defer wg.Done()

			rw.time(c)
		}(rws[i])
	}

	wg.Wait()
	close(c)

	for t := range c {
		timings = append(timings, t)
	}

	sort.Slice(timings, func(i, j int) bool {
		return timings[i].region < timings[j].region
	})

	return
}

type requestWrapper struct {
	request    *http.Request
	regionCode string
}

func wrapRequestForRegion(ctx context.Context, regionCode string, url *url.URL) (rw *requestWrapper, err error) {
	var payload = struct {
		URL    string `json:"url"`
		Region string `json:"region"`
	}{
		URL:    url.String(),
		Region: regionCode,
	}

	var buf bytes.Buffer
	if err = json.NewEncoder(&buf).Encode(payload); err != nil {
		return
	}

	var r *http.Request
	if r, err = http.NewRequestWithContext(ctx, http.MethodPost, "https://curl.fly.dev/timings", &buf); err != nil {
		return
	}

	r.Header.Add("Authorization", "1q2w3e4r")
	r.Header.Add("Content-Type", "application/json")

	rw = &requestWrapper{
		request:    r,
		regionCode: regionCode,
	}

	return
}

var httpClient = &http.Client{
	Timeout: time.Second * 3,
}

func (rw *requestWrapper) time(c chan<- *timing) {
	t := &timing{
		region: rw.regionCode,
	}
	defer func() {
		c <- t
	}()

	res, err := httpClient.Do(rw.request)
	if err != nil {
		t.error = err

		return
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		if body, err := ioutil.ReadAll(res.Body); err == nil {
			t.error = errors.New(string(body))
		} else {
			t.error = err
		}

		return
	}

	if err := json.NewDecoder(res.Body).Decode(t); err != nil {
		t.error = fmt.Errorf("failed decoding response for %s: %w", rw.regionCode, err)
	}
}

func renderTimings(w io.Writer, timings []*timing) {
	var rows [][]string
	for _, t := range timings {
		if t.error != nil {
			continue
		}

		rows = append(rows, []string{
			t.region,
			t.formatedHTTPCode(),
			t.formattedDNS(),
			t.formattedConnect(),
			t.formattedTLS(),
			t.formattedTTFB(),
			t.formattedTotal(),
		})
	}

	render.Table(w, "", rows, "Region", "Status", "DNS", "Connect", "TLS", "TTFB", "Total")

	rows = rows[:0]
	for _, t := range timings {
		if t.error == nil {
			continue
		}

		rows = append(rows, []string{
			t.region,
			t.Error(),
		})
	}

	if len(rows) == 0 {
		return
	}

	render.Table(w, "Failures", rows, "Region", "Error")
}

type timing struct {
	error
	region string

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
}

func (t *timing) formatedHTTPCode() string {
	text := strconv.Itoa(t.HTTPCode)
	return colorize(text, float64(t.HTTPCode), 299, 399)
}

func (t *timing) formattedDNS() string {
	return humanize.FtoaWithDigits(t.TimeNameLookup*1000, 1) + "ms"
}

func (t *timing) formattedConnect() string {
	timing := t.TimeConnect * 1000
	text := humanize.FtoaWithDigits(timing, 1) + "ms"
	return colorize(text, timing, 200, 500)
}

func (t *timing) formattedTLS() string {
	return humanize.FtoaWithDigits((t.TimeAppConnect+t.TimePreTransfer)*1000, 1) + "ms"
}

func (t *timing) formattedTTFB() string {
	timing := t.TimeStartTransfer * 1000
	text := humanize.FtoaWithDigits(timing, 1) + "ms"
	return colorize(text, timing, 400, 1000)
}

func (t *timing) formattedTotal() string {
	timing := t.TimeTotal * 1000
	return humanize.FtoaWithDigits(timing, 1) + "ms"
}

func colorize(text string, val float64, greenCutoff float64, yellowCutoff float64) string {
	var color aurora.Color
	switch {
	case val <= greenCutoff:
		color = aurora.GreenFg
	case val <= yellowCutoff:
		color = aurora.YellowFg
	default:
		color = aurora.RedFg
	}

	return aurora.Colorize(text, color).String()
}
