package flypkgs

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/pkg/errors"
)

func NewClient(endpoint, apiKey string) *Client {
	return &Client{
		BaseURL: endpoint,
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

	// // Uncomment this to print the response body for debugging
	// var buf bytes.Buffer
	// io.Copy(&buf, res.Body)
	// fmt.Println(buf.String())
	// res.Body = io.NopCloser(&buf)

	if res.StatusCode < http.StatusOK || res.StatusCode >= http.StatusBadRequest {
		errRes := ErrorResponse{
			Code: res.StatusCode,
		}

		if err = json.NewDecoder(res.Body).Decode(&errRes); err != nil {
			return errors.Wrap(err, "decoding error response")
		}

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
