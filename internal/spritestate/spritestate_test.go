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
	"time"

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

func TestUpdateKeepsConcurrentChanges(t *testing.T) {
	dir := t.TempDir()

	// Many writers each add their own token. A plain load-modify-save would
	// drop some; none may be lost here.
	const writers = 12

	errs := make(chan error, writers)

	for i := range writers {
		go func() {
			errs <- Update(context.Background(), dir, func(st *State) {
				if st.Tokens == nil {
					st.Tokens = map[string]string{}
				}

				st.Tokens[TokenKey("https://api.example", strings.Repeat("o", i+1))] = "tok"
			})
		}()
	}

	for range writers {
		require.NoError(t, <-errs)
	}

	st, err := Load(dir)
	require.NoError(t, err)
	assert.Len(t, st.Tokens, writers)

	// No temporary files are left next to the state file.
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)

	for _, entry := range entries {
		assert.NotContains(t, entry.Name(), ".tmp")
	}
}

func TestLockMintIsExclusive(t *testing.T) {
	dir := t.TempDir()

	unlock, err := LockMint(context.Background(), dir)
	require.NoError(t, err)

	// A second caller waits, and gives up when its context does.
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	_, err = LockMint(ctx, dir)
	require.ErrorContains(t, err, "timed out waiting")

	// Once released, the lock can be taken again.
	unlock()

	unlock, err = LockMint(context.Background(), dir)
	require.NoError(t, err)
	unlock()
}

func TestMintTokenErrors(t *testing.T) {
	replies := map[string]func(w http.ResponseWriter){
		"/v1/organizations/denied/tokens": func(w http.ResponseWriter) {
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, `{"error":"forbidden","message":"not a member of this organization"}`)
		},
		"/v1/organizations/coded/tokens": func(w http.ResponseWriter) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error":"invalid_org"}`)
		},
		"/v1/organizations/plain/tokens": func(w http.ResponseWriter) {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = io.WriteString(w, "upstream down")
		},
		"/v1/organizations/empty/tokens": func(w http.ResponseWriter) {
			_, _ = io.WriteString(w, `{}`)
		},
		"/v1/organizations/garbled/tokens": func(w http.ResponseWriter) {
			_, _ = io.WriteString(w, `not json`)
		},
	}

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		replies[r.URL.Path](w)
	}))
	defer api.Close()

	for org, want := range map[string]string{
		"denied":  "not a member of this organization (status 403)",
		"coded":   "invalid_org (status 400)",
		"plain":   "status 502: upstream down",
		"empty":   "returned no token",
		"garbled": "failed to read the token response",
	} {
		_, err := MintToken(context.Background(), api.URL, org, "fm2_login")
		require.ErrorContains(t, err, want, org)
	}

	// An unreachable API is an error too, not a hang or a panic.
	api.Close()

	_, err := MintToken(context.Background(), api.URL, "acme", "fm2_login")
	require.Error(t, err)

	require.Error(t, RevokeToken(context.Background(), api.URL, "acme", "fm2_login", "tok"))
}

func TestRevokeTokenTreatsMissingAsRevoked(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer api.Close()

	require.NoError(t, RevokeToken(context.Background(), api.URL, "acme", "fm2_login", "already-gone"))
}
