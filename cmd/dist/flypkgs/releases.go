package flypkgs

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"time"
)

type Release struct {
	ID          uint64    `json:"id"`
	Channel     string    `json:"channel"`
	Version     string    `json:"version"`
	GitCommit   string    `json:"git_commit"`
	GitBranch   string    `json:"git_branch"`
	GitRef      string    `json:"git_ref"`
	Source      string    `json:"source"`
	Status      string    `json:"status"`
	InsertedAt  time.Time `json:"inserted_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	PublishedAt time.Time `json:"published_at"`
}

func (c *Client) GetReleaseByVersion(ctx context.Context, id string) (*Release, error) {
	req, err := http.NewRequest("GET", c.URL("/releases/v:%d", id), nil)
	if err != nil {
		return nil, err
	}

	res := Release{}
	if err := c.sendRequest(ctx, req, &res); err != nil {
		return nil, err
	}

	return &res, nil
}

type CreateReleaseInput struct {
	Channel   string `json:"channel"`
	Version   string `json:"version"`
	GitCommit string `json:"git_commit"`
	GitBranch string `json:"git_branch"`
	GitRef    string `json:"git_ref"`
	Source    string `json:"source"`
	Status    string `json:"status"`
}

func (c *Client) CreateRelease(ctx context.Context, input CreateReleaseInput) (*Release, error) {
	var body bytes.Buffer
	if err := json.NewEncoder(&body).Encode(input); err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", c.URL("/releases"), &body)
	if err != nil {
		return nil, err
	}
	// req.Header.Set("Content-Type", "application/json")

	res := Release{}
	if err := c.sendRequest(ctx, req, &res); err != nil {
		return nil, err
	}

	return &res, nil

	// client := &http.Client{}
	// resp, err := client.Do(req)
	// if err != nil {
	// 	fmt.Println(err)
	// 	return err
	// }
	// defer resp.Body.Close()

	// // Read the response body
	// respBody, err := io.ReadAll(resp.Body)
	// if err != nil {
	// 	fmt.Println(err)
	// 	return err
	// }

	// fmt.Println(resp.StatusCode)

	// // switch {
	// // case resp.StatusCode >= 200 && resp.StatusCode < 300:
	// // 	return nil
	// // case resp.StatusCode == 422:
	// fmt.Println(string(respBody))
	// // }

	// return nil
}
