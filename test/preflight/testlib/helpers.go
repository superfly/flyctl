//go:build integration
// +build integration

package testlib

import (
	"crypto/md5"
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

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

func currentRepoFlyctl() string {
	_, filename, _, _ := runtime.Caller(0)
	flyctlBin := path.Join(path.Dir(filename), "../..", "bin", "flyctl")
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
	maxLen := 104 - len(filepath.Join(".fly", "fly-agent.sock"))
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

func tryToStopAgentsInOriginalHomeDir(t testing.TB, flyctlBin string) {
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
