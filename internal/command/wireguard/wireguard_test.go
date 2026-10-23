package wireguard

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/uiexutil"
)

// createCtx is withCtx plus API clients: the org lookup succeeds and
// createPeer stands in for the API's peer creation.
func createCtx(t *testing.T, args []string, createPeer func() (*fly.CreatedWireGuardPeer, error)) context.Context {
	t.Helper()

	ctx := withCtx(t, args)
	ctx = uiexutil.NewContextWithClient(ctx, &mock.UiexClient{})

	return flyutil.NewContextWithClient(ctx, &mock.Client{
		CreateWireGuardPeerFunc: func(context.Context, string, string, string, string, string) (*fly.CreatedWireGuardPeer, error) {
			return createPeer()
		},
	})
}

// The peer's private key exists only in this process. With nowhere to write
// it, the peer must not be created at all.
func TestRunWireguardCreateChecksOutputBeforeCreatingPeer(t *testing.T) {
	created := false
	ctx := createCtx(t, []string{"my-org", "ord", "peer1"}, func() (*fly.CreatedWireGuardPeer, error) {
		created = true
		return &fly.CreatedWireGuardPeer{}, nil
	})

	err := runWireguardCreate(ctx)

	if !prompt.IsNonInteractive(err) {
		t.Fatalf("expected an error naming the file argument, got %v", err)
	}
	if created {
		t.Fatal("created a WireGuard peer whose private key had nowhere to go")
	}
}

// When the peer can't be created, the file opened for its configuration is
// removed, so a retry can use the same name.
func TestRunWireguardCreateRemovesFileWhenCreateFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "peer1.conf")
	ctx := createCtx(t, []string{"my-org", "ord", "peer1", path}, func() (*fly.CreatedWireGuardPeer, error) {
		return nil, errors.New("api unavailable")
	})

	if err := runWireguardCreate(ctx); err == nil {
		t.Fatal("expected the API error")
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("left %s behind after the peer failed to create: %v", path, err)
	}
}
