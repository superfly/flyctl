package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"

	genq "github.com/Khan/genqlient/graphql"
	"github.com/superfly/graphql"
	"golang.org/x/exp/slices"
)

var (
	baseURL  string
	errorLog bool
)

// SetBaseURL - Sets the base URL for the API
func SetBaseURL(url string) {
	baseURL = url
}

// SetErrorLog - Sets whether errors should be loddes
func SetErrorLog(log bool) {
	errorLog = log
}

// Client - API client encapsulating the http and GraphQL clients
type Client struct {
	httpClient  *http.Client
	client      *graphql.Client
	GenqClient  genq.Client
	accessToken string
	logger      Logger
}

// NewClient - creates a new Client, takes an access token
func NewClient(accessToken, name, version string, logger Logger) *Client {
	transport := &Transport{
		UnderlyingTransport: http.DefaultTransport,
		Token:               accessToken,
		EnableDebugTrace:    !slices.Contains([]string{"", "0", "false"}, os.Getenv("FLY_FORCE_TRACE")),
		UserAgent:           fmt.Sprintf("%s/%s", name, version),
	}

	httpClient, _ := NewHTTPClient(logger, transport)

	url := fmt.Sprintf("%s/graphql", baseURL)

	client := graphql.NewClient(url, graphql.WithHTTPClient(httpClient))

	genqClient := genq.NewClient(url, httpClient)

	return &Client{httpClient, client, genqClient, accessToken, logger}
}

// NewRequest - creates a new GraphQL request
func (*Client) NewRequest(q string) *graphql.Request {
	q = compactQueryString(q)
	return graphql.NewRequest(q)
}

// Run - Runs a GraphQL request
func (c *Client) Run(req *graphql.Request) (Query, error) {
	return c.RunWithContext(context.Background(), req)
}

// RunWithContext - Runs a GraphQL request within a Go context
func (c *Client) RunWithContext(ctx context.Context, req *graphql.Request) (Query, error) {
	var resp Query
	err := c.client.Run(ctx, req, &resp)

	if resp.Errors != nil && errorLog {
		fmt.Fprintf(os.Stderr, "Error: %+v\n", resp.Errors)
	}

	return resp, err
}

var compactPattern = regexp.MustCompile(`\s+`)

func compactQueryString(q string) string {
	q = strings.TrimSpace(q)
	return compactPattern.ReplaceAllString(q, " ")
}

// GetAccessToken - uses email, password and possible otp to get token
func GetAccessToken(ctx context.Context, email, password, otp string) (token string, err error) {
	var postData bytes.Buffer
	if err = json.NewEncoder(&postData).Encode(map[string]interface{}{
		"data": map[string]interface{}{
			"attributes": map[string]string{
				"email":    email,
				"password": password,
				"otp":      otp,
			},
		},
	}); err != nil {
		return
	}

	url := fmt.Sprintf("%s/api/v1/sessions", baseURL)

	var req *http.Request
	if req, err = http.NewRequestWithContext(ctx, http.MethodPost, url, &postData); err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")

	var res *http.Response
	if res, err = http.DefaultClient.Do(req); err != nil {
		return
	}
	defer func() {
		closeErr := res.Body.Close()
		if err == nil {
			err = closeErr
		}
	}()

	switch {
	case res.StatusCode >= http.StatusInternalServerError:
		err = errors.New("An unknown server error occurred, please try again")
	case res.StatusCode >= http.StatusBadRequest:
		err = errors.New("Incorrect email and password combination")
	default:
		var result map[string]map[string]map[string]string

		if err = json.NewDecoder(res.Body).Decode(&result); err == nil {
			token = result["data"]["attributes"]["access_token"]
		}
	}

	return
}

type Transport struct {
	UnderlyingTransport http.RoundTripper
	UserAgent           string
	Token               string
	EnableDebugTrace    bool
}

func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("Authorization", AuthorizationHeader(t.Token))
	req.Header.Set("User-Agent", t.UserAgent)
	if t.EnableDebugTrace {
		req.Header.Set("Fly-Force-Trace", "true")
	}
	return t.UnderlyingTransport.RoundTrip(req)
}
