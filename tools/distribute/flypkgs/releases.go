package flypkgs

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"

	"github.com/superfly/flyctl/internal/version"
)

func (c *Client) GetReleaseByVersion(ctx context.Context, v version.Version) (*Release, error) {
	req, err := http.NewRequest("GET", c.URL("/releases/version/%s", v.String()), nil)
	if err != nil {
		return nil, err
	}

	res := Release{}
	if err := c.sendRequest(ctx, req, &res); err != nil {
		return nil, err
	}

	return &res, nil
}

func (c *Client) UploadRelease(ctx context.Context, v version.Version, r io.Reader) (*Release, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("file", "release.tar.gz")
	if err != nil {
		return nil, err
	}
	_, err = io.Copy(part, r)
	if err != nil {
		return nil, err
	}

	err = writer.Close()
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", c.URL("/releases/%s", v.String()), body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Accept", "application/json; charset=utf-8")

	var out Release
	if err := c.sendRequest(ctx, req, &out); err != nil {
		return nil, err
	}

	return &out, nil
}

func (c *Client) PublishRelease(ctx context.Context, v version.Version) (*Release, error) {
	req, err := http.NewRequest("POST", c.URL("/releases/%s/publish", v.String()), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json; charset=utf-8")

	var out Release
	if err := c.sendRequest(ctx, req, &out); err != nil {
		return nil, err
	}

	return &out, nil
}
