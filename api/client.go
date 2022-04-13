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

	"github.com/superfly/graphql"
)

var baseURL string
var errorLog bool

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
	accessToken string
	userAgent   string
	logger      Logger
}

// NewClient - creates a new Client, takes an access token
func NewClient(accessToken, name, version string, logger Logger) *Client {

	httpClient, _ := newHTTPClient(logger)

	url := fmt.Sprintf("%s/graphql", baseURL)

	client := graphql.NewClient(url, graphql.WithHTTPClient(httpClient))
	userAgent := fmt.Sprintf("%s/%s", name, version)
	return &Client{httpClient, client, accessToken, userAgent, logger}
}

// NewRequest - creates a new GraphQL request
func (c *Client) NewRequest(q string) *graphql.Request {
	q = compactQueryString(q)
	return graphql.NewRequest(q)
}

// Run - Runs a GraphQL request
func (c *Client) Run(req *graphql.Request) (Query, error) {
	return c.RunWithContext(context.Background(), req)
}

// RunWithContext - Runs a GraphQL request within a Go context
func (c *Client) RunWithContext(ctx context.Context, req *graphql.Request) (Query, error) {
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.accessToken))
	req.Header.Set("User-Agent", c.userAgent)

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
	defer res.Body.Close()

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
