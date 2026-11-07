package launchdarkly

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
)

// serveFlags points evalURL at a server answering every evaluation with
// flags, and records the context each request evaluated.
func serveFlags(t *testing.T, flags map[string]FeatureFlag) *[]ldcontext.Context {
	t.Helper()

	var seen []ldcontext.Context

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, b64, ok := strings.Cut(r.URL.Path, "/contexts/")
		if !ok {
			t.Errorf("unexpected path %q", r.URL.Path)
			http.NotFound(w, r)

			return
		}

		raw, err := base64.URLEncoding.DecodeString(b64)
		if err != nil {
			t.Errorf("decoding context: %v", err)
		}

		var ldc ldcontext.Context
		if err := json.Unmarshal(raw, &ldc); err != nil {
			t.Errorf("parsing context: %v", err)
		}
		seen = append(seen, ldc)

		_ = json.NewEncoder(w).Encode(flags)
	}))
	t.Cleanup(srv.Close)

	prev := evalURL
	evalURL = srv.URL + "/sdk/evalx"
	t.Cleanup(func() { evalURL = prev })

	return &seen
}

func TestWireGuardTokenModeEnabled(t *testing.T) {
	cases := map[string]struct {
		flags map[string]FeatureFlag
		want  bool
	}{
		"on":       {flags: map[string]FeatureFlag{WireGuardTokenModeFlag: {Value: true}}, want: true},
		"off":      {flags: map[string]FeatureFlag{WireGuardTokenModeFlag: {Value: false}}, want: false},
		"unset":    {flags: map[string]FeatureFlag{"something-else": {Value: true}}, want: false},
		"mistyped": {flags: map[string]FeatureFlag{WireGuardTokenModeFlag: {Value: "yes"}}, want: false},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			serveFlags(t, tc.flags)

			c := NewUserClient(7)

			if c.WireGuardTokenModeEnabled() {
				t.Error("flag on before any Refresh")
			}

			if err := c.Refresh(context.Background()); err != nil {
				t.Fatalf("Refresh: %v", err)
			}

			if got := c.WireGuardTokenModeEnabled(); got != tc.want {
				t.Errorf("WireGuardTokenModeEnabled() = %t, want %t", got, tc.want)
			}
		})
	}
}

func TestNewUserClientContext(t *testing.T) {
	seen := serveFlags(t, nil)

	if err := NewUserClient(4321).Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	if len(*seen) != 1 {
		t.Fatalf("evaluated %d contexts, want 1", len(*seen))
	}

	user := (*seen)[0].IndividualContextByKind(ldcontext.Kind("user"))
	if !user.IsDefined() {
		t.Fatal("no user context sent")
	}

	if got := user.GetValue("id").IntValue(); got != 4321 {
		t.Errorf("user id = %d, want 4321", got)
	}

	// rules target users by this key
	if got := user.Key(); got != "user-4321" {
		t.Errorf("user key = %q, want user-4321", got)
	}
}

func TestNewContextUnknownUserKeepsSharedKey(t *testing.T) {
	ldc, err := newContext(UserInfo{OrganizationID: "17"})
	if err != nil {
		t.Fatal(err)
	}

	user := ldc.IndividualContextByKind(ldcontext.Kind("user"))
	if got := user.Key(); got != "flyctl" {
		t.Errorf("unknown user key = %q, want flyctl", got)
	}

	org := ldc.IndividualContextByKind(ldcontext.Kind("organization"))
	if got := org.GetValue("id").IntValue(); got != 17 {
		t.Errorf("org id = %d, want 17", got)
	}
}

func TestRefreshFailureKeepsDefaults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	prev := evalURL
	evalURL = srv.URL
	t.Cleanup(func() { evalURL = prev })

	c := NewUserClient(1)
	if err := c.Refresh(context.Background()); err == nil {
		t.Error("Refresh succeeded against a failing server")
	}

	if c.WireGuardTokenModeEnabled() {
		t.Error("flag on after a failed Refresh")
	}
}
