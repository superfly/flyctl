package ping

import (
	"os"
	"syscall"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSendProbesSendsCount(t *testing.T) {
	for _, count := range []int{1, 3} {
		synctest.Test(t, func(t *testing.T) {
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()

			var seqs []int
			start := time.Now()
			require.NoError(t, sendProbes(count, ticker.C, nil, 2*time.Second, func(seq int) error {
				seqs = append(seqs, seq)

				return nil
			}))

			var want []int
			for seq := range count {
				want = append(want, seq)
			}
			require.Equal(t, want, seqs)
			require.Equal(t, time.Duration(count)*time.Second+2*time.Second, time.Since(start), "waits for the last reply")
		})
	}
}

func TestSendProbesUntilStopped(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		stop := make(chan os.Signal, 1)
		sent := 0
		require.NoError(t, sendProbes(0, ticker.C, stop, time.Hour, func(int) error {
			if sent++; sent == 5 {
				stop <- syscall.SIGINT
			}

			return nil
		}))
		require.Equal(t, 5, sent)
	})
}
