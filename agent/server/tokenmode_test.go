package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"strconv"
	"testing"

	genq "github.com/Khan/genqlient/graphql"
	"github.com/superfly/flyctl/internal/mock"
)

// viewer is a GraphQL client answering every query with the given viewer
// JSON, or failing with err.
type viewer struct {
	json string
	err  error
}

func (v viewer) MakeRequest(_ context.Context, _ *genq.Request, resp *genq.Response) error {
	if v.err != nil {
		return v.err
	}

	return json.Unmarshal([]byte(`{"viewer":`+v.json+`}`), resp.Data)
}

func apiClient(v viewer) *mock.Client {
	return &mock.Client{GenqClientFunc: func() genq.Client { return v }}
}

func TestTokenModeFor(t *testing.T) {
	user := func(id int) viewer {
		return viewer{json: `{"__typename":"User","internalNumericId":` + strconv.Itoa(id) + `}`}
	}
	lookupFails := viewer{err: errors.New("boom")}
	macaroon := viewer{json: `{"__typename":"Macaroon"}`}
	flag := func(on bool, err error) func(context.Context, int) (bool, error) {
		return func(context.Context, int) (bool, error) { return on, err }
	}

	cases := map[string]struct {
		local bool
		user  viewer
		flag  func(context.Context, int) (bool, error)
		want  bool
	}{
		"flag on, local off": {
			user: user(42), flag: flag(true, nil),
			want: true,
		},
		"flag on, local on": {
			local: true, user: user(42), flag: flag(true, nil),
			want: true,
		},
		"flag off, local off": {
			user: user(42), flag: flag(false, nil),
			want: false,
		},
		"flag off, local on": {
			local: true, user: user(42), flag: flag(false, nil),
			want: true,
		},
		"user lookup fails, local off": {
			user: lookupFails, flag: flag(true, nil),
			want: false,
		},
		"user lookup fails, local on": {
			local: true, user: lookupFails, flag: flag(false, nil),
			want: true,
		},
		"macaroon viewer, local off": {
			user: macaroon, flag: flag(true, nil),
			want: false,
		},
		"macaroon viewer, local on": {
			local: true, user: macaroon, flag: flag(false, nil),
			want: true,
		},
		"flag evaluation fails, local off": {
			user: user(42), flag: flag(true, errors.New("launchdarkly down")),
			want: false,
		},
		"flag evaluation fails, local on": {
			local: true, user: user(42), flag: flag(false, errors.New("launchdarkly down")),
			want: true,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			s := &server{
				Options:       Options{Logger: log.New(io.Discard, "", 0), TokenMode: tc.local},
				tokenModeFlag: tc.flag,
			}

			if got := s.tokenModeFor(context.Background(), apiClient(tc.user)); got != tc.want {
				t.Errorf("tokenModeFor() = %t, want %t", got, tc.want)
			}
		})
	}
}

func TestTokenModeForPassesUserID(t *testing.T) {
	var got int
	s := &server{
		Options: Options{Logger: log.New(io.Discard, "", 0)},
		tokenModeFlag: func(_ context.Context, userID int) (bool, error) {
			got = userID

			return false, nil
		},
	}

	s.tokenModeFor(context.Background(), apiClient(viewer{json: `{"__typename":"User","internalNumericId":1234}`}))

	if got != 1234 {
		t.Errorf("flag evaluated for user %d, want 1234", got)
	}
}

func TestFlagClientsCachePerUser(t *testing.T) {
	var fc flagClients

	a, b := fc.get(1), fc.get(2)
	if a == b {
		t.Fatal("different users share a client")
	}

	if fc.get(1) != a {
		t.Error("same user got a new client")
	}
}
