package machine

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/iostreams"
)

func TestShouldPageMachineListTable(t *testing.T) {
	wideTable := strings.Repeat("x", 121) + "\n"

	require.True(t, shouldPageMachineListTable(wideTable, 120))
	require.False(t, shouldPageMachineListTable(wideTable, 121))
}

func TestWriteMachineListTablePaging(t *testing.T) {
	for _, tt := range []struct {
		name        string
		interactive bool
		usePager    bool
		wantPaged   bool
	}{
		{name: "interactive", interactive: true, usePager: true, wantPaged: true},
		{name: "non-interactive", interactive: false, usePager: true, wantPaged: false},
		{name: "explicit empty pager", interactive: true, usePager: false, wantPaged: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			streams, _, out, _ := iostreams.Test()
			streams.SetStdinTTY(tt.interactive)
			streams.SetStdoutTTY(tt.interactive)

			t.Setenv("GO_WANT_MACHINE_LIST_PAGER", "1")
			if tt.usePager {
				t.Setenv("PAGER", "test-helper")
				streams.SetPager(strconv.Quote(os.Args[0]) + " -test.run=^TestMachineListPagerProcess$")
			} else {
				t.Setenv("PAGER", "")
				streams.SetPager("")
			}

			rows, headers := wideMachineListFixture()
			writeMachineListTable(streams, "example-app", rows, headers)

			require.Same(t, out, streams.Out)
			require.Equal(t, tt.wantPaged, strings.Contains(out.String(), "PAGED\n"))
			require.Contains(t, out.String(), "registry.fly.io/example-app@sha256:")
			require.Contains(t, out.String(), "fdaa:0:1234:a7b:123:4567:89ab:2")
		})
	}
}

func TestWriteMachineListTableFallsBackWhenPagerIsUnavailable(t *testing.T) {
	streams, _, out, _ := iostreams.Test()
	streams.SetStdinTTY(true)
	streams.SetStdoutTTY(true)

	t.Setenv("PAGER", "flyctl-missing-pager-for-test")
	streams.SetPager("flyctl-missing-pager-for-test")

	rows, headers := wideMachineListFixture()
	writeMachineListTable(streams, "example-app", rows, headers)

	require.Same(t, out, streams.Out)
	require.NotContains(t, out.String(), "PAGED\n")
	require.Contains(t, out.String(), "registry.fly.io/example-app@sha256:")
}

func wideMachineListFixture() ([][]string, []string) {
	return [][]string{{
			"d89123456789e8",
			"example-machine-name",
			"started",
			"2/2",
			"ord",
			"app",
			"registry.fly.io/example-app@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			"fdaa:0:1234:a7b:123:4567:89ab:2",
			"vol_1234567890abcdef",
			"2026-08-20T12:00:00Z",
			"2026-08-20T12:05:00Z",
			"app",
			"shared-cpu-1x:1024MB",
		}}, []string{
			"ID", "Name", "State", "Checks", "Region", "Role", "Image",
			"IP Address", "Volume", "Created", "Last Updated", "Process Group", "Size",
		}
}

func TestMachineListPagerProcess(t *testing.T) {
	if os.Getenv("GO_WANT_MACHINE_LIST_PAGER") != "1" {
		return
	}

	fmt.Fprintln(os.Stdout, "PAGED")
	_, _ = io.Copy(os.Stdout, os.Stdin)
	os.Exit(0)
}
