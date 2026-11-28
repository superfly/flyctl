package ping

import (
	"fmt"
	"os"
	"sync"
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

// Region lookups add targets while probes are sent; reading them must not race
// with that. Run with -race.
func TestTargetAddrsWhileTargetsChange(t *testing.T) {
	var mu sync.RWMutex
	targets := map[string]string{"fdaa::1": "app"}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := range 1000 {
			mu.Lock()
			targets[fmt.Sprintf("fdaa::%x", i+2)] = "region"
			mu.Unlock()
		}
	}()
	for range 1000 {
		require.Contains(t, targetAddrs(&mu, targets), "fdaa::1")
	}
	<-done
	require.Len(t, targetAddrs(&mu, targets), 1001)
}
