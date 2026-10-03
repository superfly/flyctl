package logs

import (
	"context"
	"testing"
	"time"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/wireguard"
)

// pagedLogs serves pages of log entries, then empty pages.
type pagedLogs struct {
	wireguard.WebClient
	pages [][]fly.LogEntry
	calls int
}

func (p *pagedLogs) GetAppLogs(ctx context.Context, appName, token, region, instanceID string) ([]fly.LogEntry, string, error) {
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}

	p.calls++
	if len(p.pages) == 0 {
		return nil, "", nil
	}

	page := p.pages[0]
	p.pages = p.pages[1:]

	return page, "next", nil
}

// A fresh or idle app has nothing in its log buffer, and `fly logs --no-tail`
// must still exit instead of polling until the caller gives up.
func TestPollNoTailReturnsOnEmptyBuffer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	client := &pagedLogs{}

	if err := Poll(ctx, make(chan LogEntry), client, &LogOptions{AppName: "app", NoTail: true}); err != nil {
		t.Fatalf("Poll() error = %v, want nil", err)
	}
	if client.calls != 1 {
		t.Fatalf("GetAppLogs called %d times, want 1", client.calls)
	}
}

func TestPollNoTailReturnsBufferedEntries(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	client := &pagedLogs{pages: [][]fly.LogEntry{{{Message: "one"}, {Message: "two"}}}}
	out := make(chan LogEntry, 2)

	if err := Poll(ctx, out, client, &LogOptions{AppName: "app", NoTail: true}); err != nil {
		t.Fatalf("Poll() error = %v, want nil", err)
	}
	close(out)

	var got []string
	for entry := range out {
		got = append(got, entry.Message)
	}
	if len(got) != 2 || got[0] != "one" || got[1] != "two" {
		t.Fatalf("got entries %q, want [one two]", got)
	}
}
