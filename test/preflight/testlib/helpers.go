//go:build integration
// +build integration

package testlib

import (
	"crypto/md5"
	"crypto/rand"
	"encoding/base32"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/jpillora/backoff"
	"github.com/pelletier/go-toml/v2"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/terminal"
)

const defaultRegion = "iad"

func primaryRegionFromEnv() string {
	regions := os.Getenv("FLY_PREFLIGHT_TEST_FLY_REGIONS")
	if regions == "" {
		terminal.Warnf("no region set with FLY_PREFLIGHT_TEST_FLY_REGIONS so using: %s", defaultRegion)
		return defaultRegion
	}
	pieces := strings.SplitN(regions, " ", 2)
	return pieces[0]
}

func otherRegionsFromEnv() []string {
	regions := os.Getenv("FLY_PREFLIGHT_TEST_FLY_REGIONS")
	if regions == "" {
		return nil
	}
	pieces := strings.Split(regions, " ")
	if len(pieces) > 1 {
		return pieces[1:]
	} else {
		return nil
	}
}

func RepositoryRoot() string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Join(path.Dir(filename), "../../..")
}

func currentRepoFlyctl() string {
	_, filename, _, _ := runtime.Caller(0)
	flyctlBin := path.Join(path.Dir(filename), "../../..", "bin", "flyctl")
	return flyctlBin
}

func randomName(t testingTWrapper, prefix string) string {
	b := make([]byte, 4)
	_, err := rand.Read(b)
	if err != nil {
		t.Fatalf("failed to read from random: %v", err)
	}
	if !strings.HasPrefix(prefix, "preflight") {
		prefix = fmt.Sprintf("preflight-%s", prefix)
	}
	randStr := base32.StdEncoding.EncodeToString(b)
	randStr = strings.Replace(randStr, "=", "z", -1)
	dateStr := time.Now().Format("2006-01")
	return fmt.Sprintf("%s-%s-%s", prefix, dateStr, strings.ToLower(randStr))
}

// fun times with sockets
// https://github.com/golang/go/issues/6895#issuecomment-66088946
func socketSafeTempDir(t testing.TB) string {
	tempDir := t.TempDir()
	maxLen := 103 - len(filepath.Join(".fly", "fly-agent.sock"))
	if len(tempDir) < maxLen {
		return tempDir
	}
	hasher := md5.New()
	hasher.Write([]byte(tempDir))
	sum := base32.StdEncoding.EncodeToString(hasher.Sum(nil))
	sum = strings.Replace(sum, "=", "", -1)
	shorterTempDir := filepath.Join(os.TempDir(), sum)
	err := os.Symlink(tempDir, shorterTempDir)
	if err != nil {
		t.Fatalf("default temp dir is too long (len %d), but we failed to create symlink to %s (len %d) because: %v", len(tempDir), shorterTempDir, len(shorterTempDir), err)
	}
	t.Cleanup(func() {
		os.Remove(shorterTempDir)
	})
	return shorterTempDir
}

func tryToStopAgentsInOriginalHomeDir(flyctlBin string) {
	testIostreams, _, _, _ := iostreams.Test()
	cmd := exec.Command(flyctlBin, "agent", "stop")
	cmd.Stdin = testIostreams.In
	cmd.Stdout = testIostreams.Out
	cmd.Stderr = testIostreams.ErrOut
	err := cmd.Start()
	if err != nil {
		return
	}
	_ = cmd.Wait()
}

func tryToStopAgentsFromPastPreflightTests(t testing.TB, flyctlBin string) {
	// FIXME: make something like ps au | grep flyctl | grep $TMPDIR | grep agent, then kill those procs?
}

// RunHealthcheck verifies if an app was deployed successfully.
// It runs the checks 10 times with some backoff.
func RunHealthCheck(url string) (string, error) {
	var (
		resp *http.Response
		err  error
	)

	lastStatusCode, attempts := -1, 10
	b := &backoff.Backoff{
		Factor: 2,
		Jitter: true,
		Min:    100 * time.Millisecond,
		Max:    5 * time.Second,
	}

	for i := 0; i < attempts; i++ {
		resp, err = http.Get(url)
		if err == nil {
			lastStatusCode = resp.StatusCode
		}
		if lastStatusCode == http.StatusOK {
			break
		}

		time.Sleep(b.Duration())
	}

	if err != nil {
		return "", err
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}

func castEnv(raw any) (map[string]string, error) {
	env := map[string]string{}

	switch cast := raw.(type) {
	case map[string]string:
		env = cast
	case []map[string]any:
		for _, raw2 := range cast {
			env2, err := castEnv(raw2)
			if err != nil {
				return nil, err
			}
			for k, v := range env2 {
				env[k] = v
			}
		}
	case []any:
		for _, raw2 := range cast {
			env2, err := castEnv(raw2)
			if err != nil {
				return nil, err
			}
			for k, v := range env2 {
				env[k] = v
			}
		}
	case map[string]any:
		for k, v := range cast {
			if stringVal, ok := v.(string); ok {
				env[k] = stringVal
			} else {
				env[k] = fmt.Sprintf("%v", v)
			}
		}
	default:
		fmt.Println(cast)
		return nil, errors.New("failed to cast 'env'")
	}

	return env, nil
}

// LoadConfig loads the app config at the given path.
func readToml(path string) (map[string]any, error) {
	buf, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	def := map[string]any{}
	err = toml.Unmarshal(buf, &def)
	if err != nil {
		return nil, err
	}

	return def, err
}

func writeToml(path string, data map[string]any) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	err = toml.NewEncoder(file).Encode(data)
	if err != nil {
		return err
	}

	return nil
}

// OverwriteConfig overwrites app config fields with the given data.
func OverwriteConfig(path string, data map[string]any) error {
	cfg, err := readToml(path)
	if err != nil {
		return err
	}

	cfgEnv, err := castEnv(cfg["env"])
	if err != nil {
		return err
	}

	dataEnv, err := castEnv(data["env"])
	if err != nil {
		return err
	}

	// merge the 2 "envs"
	for k, v := range dataEnv {
		cfgEnv[k] = v
	}

	cfg["app"] = data["app"]
	cfg["env"] = cfgEnv
	cfg["primary_region"] = data["region"]

	err = writeToml(path, cfg)
	if err != nil {
		return err
	}

	return nil
}
