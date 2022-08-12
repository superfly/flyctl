package diag

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/build/imgsrc"
	"github.com/superfly/flyctl/internal/buildinfo"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/state"
	"github.com/superfly/flyctl/iostreams"
)

var urlrx = regexp.MustCompile(`https://.*?/[0-9]+/([a-z0-9-]+).zip\\?`)

func New() (cmd *cobra.Command) {
	var (
		short = `Send diagnostic information about your applications back to Fly.io.`
		long  = `Send diagnostic information about your applications back to Fly.io,
to help diagnose problems.

This command will collect some local system information and a few files
that you'd be sending us anyways in order to deploy, notably any Dockerfiles
you might have associated with this application.
`
	)

	cmd = command.New("diag", short, long, Run,
		command.RequireSession,
		command.LoadAppNameIfPresent,
	)

	cmd.Args = cobra.NoArgs

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.Bool{
			Name:        "force",
			Default:     false,
			Description: "Send diagnostics even if we can't find your local Fly.io app",
		},
	)

	return
}

func Run(ctx context.Context) (err error) {
	var (
		isForce = flag.GetBool(ctx, "force")
		ios     = iostreams.FromContext(ctx)
		color   = ios.ColorScheme()
	)

	_, err = os.Stat("./fly.toml")
	if err != nil && !isForce {
		fmt.Printf(`Can't find "fly.toml" in your local directory.

Run this command from your app's local directory, or
add the --force flag to send us best-effort diagnostics.`)
		return err
	}

	fts := []struct {
		name   string
		fn     func(context.Context, *zip.Writer) error
		expect bool
	}{
		{"fly.toml", fetchFlyToml, true},
		{"config.yml", fetchConfigYaml, true},
		{"Dockerfile", fetchDockerfile, false},
		{"fly agent logs", fetchAgentLogs, false},
		{"local diagnostics", fetchLocalDiag, true},
	}

	zbuf := &bytes.Buffer{}
	zip := zip.NewWriter(zbuf)

	for _, ft := range fts {
		fmt.Printf("Collecting %s... ", ft.name)

		if err = ft.fn(ctx, zip); err != nil {
			if ft.expect {
				fmt.Printf(color.Red(fmt.Sprintf("FAILED: %s\n", err)))
			} else {
				fmt.Printf("skipping\n")
			}
		} else {
			fmt.Printf(color.Green("ok\n"))
		}
	}

	zip.Close()

	client := client.FromContext(ctx).API()

	url, err := client.CreateDoctorUrl(context.Background())
	if err != nil {
		return fmt.Errorf("create doctor URL: %w", err)
	}

	m := urlrx.FindStringSubmatch(url)
	if m == nil {
		return fmt.Errorf("malformed S3 URL (this is a bug, tell us)")
	}

	req, err := http.NewRequest(http.MethodPut, url, zbuf)
	if err != nil {
		return fmt.Errorf("request doctor URL: %w", err)
	}

	req.Header.Set("Content-Type", "application/zip")

	_, err = http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("put archive to doctor URL: %w", err)
	}

	fmt.Printf("\nYour Diagnostic Code (safe to share): %s\n", m[1])

	return nil
}

func cp(z *zip.Writer, name string, f io.Reader) error {
	zf, err := z.Create(name)
	if err != nil {
		return err
	}

	_, err = io.Copy(zf, f)
	return err
}

func fetchFlyToml(ctx context.Context, z *zip.Writer) error {
	f, err := os.Open("fly.toml")
	if err != nil {
		return err
	}
	defer f.Close()

	return cp(z, "fly.toml", f)
}

func grepv(r io.Reader, excludes []string) *bytes.Buffer {
	buf := &bytes.Buffer{}
	rdr := bufio.NewReader(r)

	for {
		line, err := rdr.ReadString('\n')
		if err != nil {
			break
		}

		exclude := false
		for _, xcl := range excludes {
			if strings.Contains(line, xcl) {
				exclude = true
				break
			}
		}

		if !exclude {
			buf.WriteString(line)
		}
	}

	return buf
}

func fetchConfigYaml(ctx context.Context, z *zip.Writer) error {
	path := filepath.Join(state.ConfigDirectory(ctx), config.FileName)
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	return cp(z, "config.yml", grepv(f, []string{"_token:", "private"}))
}

func fetchAgentLogs(ctx context.Context, z *zip.Writer) error {
	logs, err := filepath.Glob(fmt.Sprintf("%s/.fly/agent-logs/*.log", os.Getenv("HOME")))
	if err != nil {
		// swallow this error, agent logs are best-effort
		return nil
	}

	for _, logfile := range logs {
		f, err := os.Open(logfile)
		if err != nil {
			continue
		}

		cp(z, "agent/"+path.Base(logfile),
			grepv(f, []string{"private", "password" /* who knows, whatever */}))
	}

	return nil
}

func fetchDockerfile(ctx context.Context, z *zip.Writer) error {
	f, err := os.Open("Dockerfile")
	if err != nil {
		return err
	}
	defer f.Close()

	return cp(z, "Dockerfile", f)
}

func fetchLocalDiag(ctx context.Context, z *zip.Writer) error {
	diags := map[string]interface{}{}

	diags["version"] = buildinfo.Info()

	client, err := imgsrc.NewLocalDockerClient()
	if err == nil {
		_, err = client.Ping(ctx)
	}
	if err == nil {
		diags["docker"] = "present"
	} else {
		diags["docker"] = err.Error()
	}

	if _, err = os.Stat("fly.toml"); err == nil {
		var size int64
		err = filepath.Walk(".", func(_ string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() {
				size += info.Size()
			}
			return err
		})

		diags["treesize"] = size
		if err != nil {
			diags["tree_errs"] = err.Error()
		}
	}

	buf := &bytes.Buffer{}
	json.NewEncoder(buf).Encode(diags)
	return cp(z, "diag.json", buf)
}
