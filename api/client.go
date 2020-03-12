package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/machinebox/graphql"
)

var baseURL string

func SetBaseURL(url string) {
	baseURL = url
}

type Client struct {
	httpClient  *http.Client
	client      *graphql.Client
	accessToken string
	userAgent   string
}

func NewClient(accessToken string, version string) *Client {
	if accessToken == "" {
		panic("Access token cannot be blank")
	}

	httpClient, _ := newHTTPClient()

	url := fmt.Sprintf("%s/api/v2/graphql", baseURL)

	client := graphql.NewClient(url, graphql.WithHTTPClient(httpClient))
	userAgent := fmt.Sprintf("flyctl/%s", version)
	return &Client{httpClient, client, accessToken, userAgent}
}

func (c *Client) NewRequest(q string) *graphql.Request {
	q = compactQueryString(q)
	return graphql.NewRequest(q)
}

func (c *Client) Run(req *graphql.Request) (Query, error) {
	return c.RunWithContext(context.Background(), req)
}

func (c *Client) RunWithContext(ctx context.Context, req *graphql.Request) (Query, error) {
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.accessToken))
	req.Header.Set("User-Agent", c.userAgent)

	var resp Query
	err := c.client.Run(ctx, req, &resp)
	if err != nil && strings.HasPrefix(err.Error(), "graphql: ") {
		return resp, errors.New(strings.TrimPrefix(err.Error(), "graphql: "))
	}
	return resp, err
}

var compactPattern = regexp.MustCompile(`\s+`)

func compactQueryString(q string) string {
	q = strings.TrimSpace(q)
	return compactPattern.ReplaceAllString(q, " ")
}

func GetAccessToken(email, password, otp string) (string, error) {
	postData, _ := json.Marshal(map[string]interface{}{
		"data": map[string]interface{}{
			"attributes": map[string]string{
				"email":    email,
				"password": password,
				"otp":      otp,
			},
		},
	})

	url := fmt.Sprintf("%s/api/v1/sessions", baseURL)

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(postData))
	if err != nil {
		return "", err
	}

	if resp.StatusCode >= 500 {
		return "", errors.New("An unknown server error occured, please try again")
	}

	if resp.StatusCode >= 400 {
		return "", errors.New("Incorrect email and password combination")
	}

	defer resp.Body.Close()

	var result map[string]map[string]map[string]string

	json.NewDecoder(resp.Body).Decode(&result)

	accessToken := result["data"]["attributes"]["access_token"]

	return accessToken, nil
}
