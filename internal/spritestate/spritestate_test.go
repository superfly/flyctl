package spritestate

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadMissingFileIsEmpty(t *testing.T) {
	st, err := Load(t.TempDir())
	require.NoError(t, err)
	assert.Equal(t, &State{}, st)
}

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()

	want := &State{
		DefaultOrg: "acme",
		Tokens:     map[string]string{TokenKey("https://api.sprites.dev", "acme"): "tok"},
	}
	require.NoError(t, Save(dir, want))

	got, err := Load(dir)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestSaveIsOwnerOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permissions")
	}

	dir := t.TempDir()
	require.NoError(t, Save(dir, &State{Tokens: map[string]string{"k": "secret"}}))

	info, err := os.Stat(Path(dir))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestLoadCorruptFileStartsOver(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(Path(dir), []byte("{not json"), 0o600))

	st, err := Load(dir)
	require.NoError(t, err)
	assert.Equal(t, &State{}, st)
}

func TestClear(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, Save(dir, &State{DefaultOrg: "acme"}))

	require.NoError(t, Clear(dir))
	assert.NoFileExists(t, Path(dir))

	// Clearing when there is nothing to clear is fine.
	require.NoError(t, Clear(dir))
}

func TestMintAndRevokeAll(t *testing.T) {
	var requests []string

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		requests = append(requests, r.Method+" "+r.URL.Path+" "+string(body))

		assert.Equal(t, "FlyV1 fm2_login", r.Header.Get("Authorization"))

		switch {
		case r.Method == http.MethodPost:
			_, _ = io.WriteString(w, `{"token":"minted"}`)
		case strings.Contains(r.URL.Path, "/broken/"):
			w.WriteHeader(http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer api.Close()

	token, err := MintToken(context.Background(), api.URL, "acme", "fm2_login")
	require.NoError(t, err)
	assert.Equal(t, "minted", token)
	assert.Equal(t, `POST /v1/organizations/acme/tokens {"description":"flyctl"}`, requests[0])

	dir := t.TempDir()
	require.NoError(t, Save(dir, &State{Tokens: map[string]string{
		TokenKey(api.URL, "acme"):   "tok-acme",
		TokenKey(api.URL, "broken"): "tok-broken",
	}}))

	failed, err := RevokeAll(context.Background(), dir, "fm2_login")
	require.NoError(t, err)

	// What could not be revoked is reported, and the file goes either way.
	assert.Equal(t, []string{"broken"}, failed)
	assert.NoFileExists(t, Path(dir))
	assert.Contains(t, requests, `DELETE /v1/organizations/acme/tokens {"token":"tok-acme"}`)
}

func TestRevokeAllWithoutLoginStillClears(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, Save(dir, &State{Tokens: map[string]string{TokenKey("https://api.example", "acme"): "tok"}}))

	failed, err := RevokeAll(context.Background(), dir, "")
	require.NoError(t, err)
	assert.Equal(t, []string{"acme"}, failed)
	assert.NoFileExists(t, Path(dir))
}
