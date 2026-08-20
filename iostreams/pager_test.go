package iostreams

import (
	"io"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPagerLifecycle(t *testing.T) {
	if os.Getenv("GO_WANT_IOSTREAMS_PAGER") == "1" {
		_, _ = io.Copy(os.Stdout, os.Stdin)
		os.Exit(0)
	}

	streams, _, out, _ := Test()
	streams.SetStdoutTTY(true)
	streams.SetPager(strconv.Quote(os.Args[0]) + " -test.run=^TestPagerLifecycle$")
	t.Setenv("GO_WANT_IOSTREAMS_PAGER", "1")

	originalOut := streams.Out
	require.NoError(t, streams.StartPager())

	payload := strings.Repeat("pager output\n", 8192)
	_, err := io.WriteString(streams.Out, payload)
	require.NoError(t, err)
	streams.StopPager()

	require.Same(t, originalOut, streams.Out)
	require.Equal(t, payload, out.String())
}
