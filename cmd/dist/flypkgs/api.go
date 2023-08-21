package flypkgs

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	BaseURL = "http://localhost:4000/api"
)

func NewClient(apiKey string) *Client {
	return &Client{
		BaseURL: BaseURL,
		apiKey:  apiKey,
		HTTPClient: &http.Client{
			Timeout: time.Minute,
		},
	}
}

type successResponse struct {
	Code int         `json:"code"`
	Data interface{} `json:"data"`
}

// var ErrNotFound = errors.New("not found")

type Client struct {
	BaseURL    string
	apiKey     string
	HTTPClient *http.Client
}

func (c *Client) URL(path string, params ...any) string {
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return fmt.Sprintf(c.BaseURL+path, params...)
}

func (c *Client) sendRequest(ctx context.Context, req *http.Request, v interface{}) error {
	req = req.WithContext(ctx)

	if len(req.Header.Values("Content-Type")) == 0 {
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
	}

	req.Header.Set("Accept", "application/json; charset=utf-8")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiKey))

	res, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}

	defer res.Body.Close()

	if res.StatusCode < http.StatusOK || res.StatusCode >= http.StatusBadRequest {
		errRes := ErrorResponse{
			Code: res.StatusCode,
		}
		if err = json.NewDecoder(res.Body).Decode(&errRes); err == nil {
			return errRes
		}

		errRes.Message = "unknown error"
		return errRes
	}

	fullResponse := successResponse{
		Data: v,
	}
	if err = json.NewDecoder(res.Body).Decode(&fullResponse); err != nil {
		return err
	}

	return nil
}
