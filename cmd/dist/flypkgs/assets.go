package flypkgs

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"time"
)

type Asset struct {
	ID          uint64 `json:"id"`
	Size        uint64 `json:"size"`
	Name        string `json:"name"`
	ContentType string `json:"content_type"`
	// DownloadURL string `json:"download_url"`
	Checksum   string    `json:"checksum"`
	OS         string    `json:"os"`
	Arch       string    `json:"arch"`
	InsertedAt time.Time `json:"inserted_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// size: asset.size,
// name: asset.name,
// content_type: asset.content_type,
// download_url: asset.download_url,
// checksum: asset.checksum,
// os: asset.os,
// arch: asset.arch

type CreateAssetInput struct {
	ReleaseID uint64 `json:"release_id"`
	Name      string `json:"name"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	Checksum  string `json:"checksum"`
}

func (c *Client) GetAssetByID(ctx context.Context, id uint64) (*Asset, error) {
	req, err := http.NewRequest("GET", c.URL("/assets/%d", id), nil)
	if err != nil {
		return nil, err
	}

	var out Asset
	if err := c.sendRequest(ctx, req, &out); err != nil {
		return nil, err
	}

	return &out, nil
}

func (c *Client) GetAsset(ctx context.Context, releaseID string, os string, arch string) (*Asset, error) {
	req, err := http.NewRequest("GET", c.URL("/releases/%s/assets/%s/%s", releaseID, os, arch), nil)
	if err != nil {
		return nil, err
	}

	var out Asset
	if err := c.sendRequest(ctx, req, &out); err != nil {
		return nil, err
	}

	return &out, nil
}

func (c *Client) CreateAsset(ctx context.Context, in CreateAssetInput, r io.Reader) (*Asset, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	if err := writer.WriteField("os", in.OS); err != nil {
		return nil, err
	}
	if err := writer.WriteField("arch", in.Arch); err != nil {
		return nil, err
	}
	if err := writer.WriteField("checksum", in.Checksum); err != nil {
		return nil, err
	}

	// Add the file to the form
	part, err := writer.CreateFormFile("file", filepath.Base(in.Name))
	if err != nil {
		fmt.Println(err)
		return nil, err
	}
	_, err = io.Copy(part, r)
	if err != nil {
		fmt.Println(err)
		return nil, err
	}

	err = writer.Close()
	if err != nil {
		fmt.Println(err)
		return nil, err
	}
	// /"+artifact.OS+"/"+artifact.Arch

	req, err := http.NewRequest("POST", c.URL("/releases/%d/assets", in.ReleaseID), body)
	if err != nil {
		// fmt.Println(err)
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	// req.Header.Set("Accepts", "application/json")

	var out Asset
	if err := c.sendRequest(ctx, req, &out); err != nil {
		return nil, err
	}

	return &out, nil
}
