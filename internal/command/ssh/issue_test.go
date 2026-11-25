package ssh

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"golang.org/x/crypto/ssh"

	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/uiex"
	"github.com/superfly/flyctl/internal/uiexutil"
	"github.com/superfly/flyctl/iostreams"
)

func runIssueForTest(t *testing.T, args ...string) error {
	t.Helper()
	t.Setenv("FLY_ORG", "")

	io, _, _, _ := iostreams.Test()
	ctx := iostreams.NewContext(t.Context(), io)
	ctx = uiexutil.NewContextWithClient(ctx, &mock.UiexClient{
		GetOrganizationFunc: func(_ context.Context, slug string) (*uiex.Organization, error) {
			require.Equal(t, "acme", slug)

			return &uiex.Organization{Slug: slug}, nil
		},
	})
	ctx = flyutil.NewContextWithClient(ctx, &mock.Client{
		IssueSSHCertificateFunc: func(context.Context, string, []string, []string, *int, ed25519.PublicKey) (*fly.IssuedCertificate, error) {
			return &fly.IssuedCertificate{Certificate: "new-cert"}, nil
		},
	})

	cmd := newIssue()
	require.NoError(t, cmd.Flags().Parse(args))

	return runSSHIssue(flag.NewContext(ctx, cmd.Flags()))
}

func requireCredential(t *testing.T, path string) {
	t.Helper()

	key, err := os.ReadFile(path)
	require.NoError(t, err)
	_, err = ssh.ParseRawPrivateKey(key)
	require.NoError(t, err)

	cert, err := os.ReadFile(path + "-cert.pub")
	require.NoError(t, err)
	require.Equal(t, "new-cert", string(cert))
}

func TestIssueOverwritesFlyKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "id_fly")
	require.NoError(t, runIssueForTest(t, "acme", path))
	first, err := os.ReadFile(path)
	require.NoError(t, err)

	// longer than the new one, so leftovers would show
	require.NoError(t, os.WriteFile(path+"-cert.pub", []byte("an old and longer certificate"), 0o600))

	require.NoError(t, runIssueForTest(t, "acme", path, "--overwrite"))
	requireCredential(t, path)
	second, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NotEqual(t, first, second)
}

func TestIssueRefusesToOverwriteOtherFiles(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	otherKey, err := ssh.MarshalPrivateKey(priv, "me@fly.io.example")
	require.NoError(t, err)

	for name, content := range map[string][]byte{
		"plain text mentioning fly.io": []byte("fly.io"),
		"another key":                  pem.EncodeToMemory(otherKey),
		"truncated":                    MarshalED25519PrivateKey(priv, "fly.io")[:80],
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "id_other")
			require.NoError(t, os.WriteFile(path, content, 0o600))

			err := runIssueForTest(t, "acme", path, "--overwrite")
			require.True(t, prompt.IsNonInteractive(err), "got %v", err)

			after, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, content, after)
			require.NoFileExists(t, path+"-cert.pub")
		})
	}
}

func TestIssueExistingCertificateLeavesNoKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "id_fly")
	require.NoError(t, os.WriteFile(path+"-cert.pub", []byte("old-cert"), 0o600))

	err := runIssueForTest(t, "acme", path)
	require.True(t, prompt.IsNonInteractive(err), "got %v", err)
	require.NoFileExists(t, path)

	cert, err := os.ReadFile(path + "-cert.pub")
	require.NoError(t, err)
	require.Equal(t, "old-cert", string(cert))
}

func TestIssueOverwriteKeepsKeyWhenCertificateFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "id_fly")
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	key := MarshalED25519PrivateKey(priv, "fly.io")
	require.NoError(t, os.WriteFile(path, key, 0o600))
	require.NoError(t, os.Mkdir(path+"-cert.pub", 0o700)) // can't be opened for writing

	err = runIssueForTest(t, "acme", path, "--overwrite")
	require.True(t, prompt.IsNonInteractive(err), "got %v", err)

	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, key, after)
}

func TestIssueUsesLoneArgumentAsPathWithOrgFlag(t *testing.T) {
	path := filepath.Join(t.TempDir(), "id_fly")
	require.NoError(t, runIssueForTest(t, "--org", "acme", path))
	requireCredential(t, path)
}
