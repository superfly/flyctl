package uiex

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"strconv"
	"time"

	"github.com/superfly/flyctl/internal/config"
)

// Deployments of apps that deploy from GitHub through the Fly.io dashboard: the
// /api/v1/apps/:app/deployments API documented at https://fly.io/openapi/deployments.json.

type Deployment struct {
	ID           int                `json:"id"`
	App          string             `json:"app"`
	Status       string             `json:"status"`
	CurrentStep  *string            `json:"current_step"`
	Trigger      *string            `json:"trigger"`
	Git          DeploymentGit      `json:"git"`
	Release      *DeploymentRelease `json:"release"`
	PlanStatus   *string            `json:"plan_status"`
	Failure      *DeploymentFailure `json:"failure"`
	CreatedAt    time.Time          `json:"created_at"`
	UpdatedAt    time.Time          `json:"updated_at"`
	DashboardURL string             `json:"dashboard_url"`

	// Only set when fetching a single deployment.
	Source *RepositorySettings `json:"source,omitempty"`
	Steps  []DeploymentStep    `json:"steps,omitempty"`
	Errors []DeploymentError   `json:"errors,omitempty"`
}

// Finished reports whether the deployment reached a final status.
func (d *Deployment) Finished() bool {
	return d.Status != "running"
}

type DeploymentGit struct {
	Repository    *string `json:"repository"`
	Ref           *string `json:"ref"`
	Commit        *string `json:"commit"`
	CommitMessage *string `json:"commit_message"`
	CommitAuthor  *string `json:"commit_author"`
}

type DeploymentRelease struct {
	Version int     `json:"version"`
	Image   *string `json:"image"`
}

type DeploymentFailure struct {
	Step     *string `json:"step"`
	Category string  `json:"category"`
	Title    string  `json:"title"`
}

type DeploymentStep struct {
	Name            string   `json:"name"`
	Description     string   `json:"description"`
	Status          string   `json:"status"`
	DurationSeconds *float64 `json:"duration_seconds"`
}

type DeploymentError struct {
	Step    string  `json:"step"`
	Message string  `json:"message"`
	Stderr  *string `json:"stderr"`
}

type RepositorySettings struct {
	Repository       string  `json:"repository"`
	Branch           *string `json:"branch"`
	AutoDeploy       bool    `json:"auto_deploy"`
	ConfigPath       *string `json:"config_path"`
	WorkingDirectory *string `json:"working_directory"`
}

type DeploymentLogLine struct {
	Step    string `json:"step"`
	Type    string `json:"type"`
	Message string `json:"message"`
}

type DeploymentLogs struct {
	DeploymentID int                 `json:"deployment_id"`
	Status       string              `json:"status"`
	Truncated    bool                `json:"truncated"`
	Lines        []DeploymentLogLine `json:"lines"`
}

type LaunchPlan struct {
	DeploymentID int              `json:"deployment_id"`
	Status       string           `json:"status"`
	Settings     map[string]any   `json:"settings"`
	Files        []LaunchPlanFile `json:"files"`
}

type LaunchPlanFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type CreateDeploymentRequest struct {
	Ref     string `json:"ref,omitempty"`
	Replace bool   `json:"replace,omitempty"`
}

type LaunchRequest struct {
	Repository       string `json:"repository"`
	Branch           string `json:"branch,omitempty"`
	ConfigPath       string `json:"config_path,omitempty"`
	WorkingDirectory string `json:"working_directory,omitempty"`
	ReviewPlan       bool   `json:"review_plan,omitempty"`
}

// ActionRequired is a step only a person can take before retrying, such as
// installing the Fly.io GitHub App.
type ActionRequired struct {
	Kind          string   `json:"kind"`
	URL           string   `json:"url"`
	FailingChecks []string `json:"failing_checks,omitempty"`
}

// DeploymentsAPIError is an error answer from the deployments API. Message is meant
// for the user.
type DeploymentsAPIError struct {
	StatusCode     int
	Message        string
	ActionRequired *ActionRequired
}

func (e *DeploymentsAPIError) Error() string {
	if e.ActionRequired != nil {
		return fmt.Sprintf("%s\n%s", e.Message, e.ActionRequired.URL)
	}
	return e.Message
}

// ListDeployments returns the app's deployments, newest first. before is a
// deployment id to page past, or 0.
func (c *Client) ListDeployments(ctx context.Context, appName string, limit, before int) ([]Deployment, error) {
	query := neturl.Values{}
	if limit > 0 {
		query.Set("limit", strconv.Itoa(limit))
	}
	if before > 0 {
		query.Set("before", strconv.Itoa(before))
	}

	var out struct {
		Data []Deployment `json:"data"`
	}
	err := c.deploymentsRequest(ctx, http.MethodGet, appPath(appName, "/deployments"), query, nil, &out)

	return out.Data, err
}

// GetDeployment returns a deployment; id may be "latest". With wait > 0 and a
// running deployment, the API waits up to wait seconds for it to change.
func (c *Client) GetDeployment(ctx context.Context, appName, id string, wait int) (*Deployment, error) {
	query := neturl.Values{}
	if wait > 0 {
		query.Set("wait", strconv.Itoa(wait))
	}

	var out struct {
		Data Deployment `json:"data"`
	}
	if err := c.deploymentsRequest(ctx, http.MethodGet, deploymentPath(appName, id, ""), query, nil, &out); err != nil {
		return nil, err
	}

	return &out.Data, nil
}

func (c *Client) GetDeploymentLogs(ctx context.Context, appName, id, step string, limit int) (*DeploymentLogs, error) {
	query := neturl.Values{}
	if step != "" {
		query.Set("step", step)
	}
	if limit > 0 {
		query.Set("limit", strconv.Itoa(limit))
	}

	var out struct {
		Data DeploymentLogs `json:"data"`
	}
	if err := c.deploymentsRequest(ctx, http.MethodGet, deploymentPath(appName, id, "/logs"), query, nil, &out); err != nil {
		return nil, err
	}

	return &out.Data, nil
}

// GetDeploymentDiagnosis returns the Markdown brief of a deployment. lines < 0
// keeps the API's default.
func (c *Client) GetDeploymentDiagnosis(ctx context.Context, appName, id string, lines int) (string, error) {
	query := neturl.Values{}
	if lines >= 0 {
		query.Set("lines", strconv.Itoa(lines))
	}

	var out bytes.Buffer
	err := c.deploymentsRequest(ctx, http.MethodGet, deploymentPath(appName, id, "/diagnosis"), query, nil, &out)

	return out.String(), err
}

func (c *Client) CreateDeployment(ctx context.Context, appName string, in CreateDeploymentRequest) (*Deployment, error) {
	return c.deploymentWrite(ctx, appPath(appName, "/deployments"), in)
}

func (c *Client) RetryDeployment(ctx context.Context, appName, id string) (*Deployment, error) {
	return c.deploymentWrite(ctx, deploymentPath(appName, id, "/retry"), nil)
}

func (c *Client) CancelDeployment(ctx context.Context, appName, id string) (*Deployment, error) {
	return c.deploymentWrite(ctx, deploymentPath(appName, id, "/cancel"), nil)
}

func (c *Client) LaunchFromRepository(ctx context.Context, appName string, in LaunchRequest) (*Deployment, error) {
	return c.deploymentWrite(ctx, appPath(appName, "/deployments/launch"), in)
}

func (c *Client) GetLaunchPlan(ctx context.Context, appName, id string) (*LaunchPlan, error) {
	var out struct {
		Data LaunchPlan `json:"data"`
	}
	if err := c.deploymentsRequest(ctx, http.MethodGet, deploymentPath(appName, id, "/plan"), nil, nil, &out); err != nil {
		return nil, err
	}

	return &out.Data, nil
}

// ConfirmLaunchPlan confirms a launch's detected plan, overriding any of region,
// http_service_port, vm_size, vm_memory, vm_cpus and vm_cpukind.
func (c *Client) ConfirmLaunchPlan(ctx context.Context, appName, id string, settings map[string]any) (*Deployment, error) {
	var body any
	if len(settings) > 0 {
		body = map[string]any{"settings": settings}
	}
	return c.deploymentWrite(ctx, deploymentPath(appName, id, "/confirm"), body)
}

func (c *Client) GetRepositorySettings(ctx context.Context, appName string) (*RepositorySettings, error) {
	var out struct {
		Data RepositorySettings `json:"data"`
	}
	if err := c.deploymentsRequest(ctx, http.MethodGet, appPath(appName, "/deployments/source"), nil, nil, &out); err != nil {
		return nil, err
	}

	return &out.Data, nil
}

func (c *Client) deploymentWrite(ctx context.Context, path string, body any) (*Deployment, error) {
	var out struct {
		Data Deployment `json:"data"`
	}
	if err := c.deploymentsRequest(ctx, http.MethodPost, path, nil, body, &out); err != nil {
		return nil, err
	}

	return &out.Data, nil
}

func appPath(appName, rest string) string {
	return "/api/v1/apps/" + neturl.PathEscape(appName) + rest
}

func deploymentPath(appName, id, rest string) string {
	return appPath(appName, "/deployments/"+neturl.PathEscape(id)+rest)
}

// deploymentsRequest sends a request and decodes a 2xx answer into out (JSON), or
// copies it into out when out is a *bytes.Buffer. Error answers become
// *DeploymentsAPIError.
func (c *Client) deploymentsRequest(ctx context.Context, method, path string, query neturl.Values, body, out any) error {
	cfg := config.FromContext(ctx)

	url := c.baseUrl.String() + path
	if len(query) > 0 {
		url += "?" + query.Encode()
	}

	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("failed to encode request: %w", err)
		}
		reader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Add("Authorization", "Bearer "+cfg.Tokens.GraphQL())
	req.Header.Add("Content-Type", "application/json")

	res, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	data, err := io.ReadAll(res.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	if res.StatusCode < 200 || res.StatusCode > 299 {
		return deploymentsError(res.StatusCode, data)
	}

	if buf, ok := out.(*bytes.Buffer); ok {
		buf.Write(data)
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("failed to decode response, please try again: %w", err)
	}

	return nil
}

func deploymentsError(status int, data []byte) error {
	var body struct {
		Error          string          `json:"error"`
		ActionRequired *ActionRequired `json:"action_required"`
	}
	_ = json.Unmarshal(data, &body)

	message := body.Error
	if message == "" {
		switch status {
		case http.StatusNotFound:
			message = "app or deployment not found, or this token can't access it"
		default:
			message = fmt.Sprintf("unexpected response (status %d): %s", status, string(data))
		}
	}

	return &DeploymentsAPIError{StatusCode: status, Message: message, ActionRequired: body.ActionRequired}
}

// IsDeploymentsNotFound reports whether err is a 404 from the deployments API.
func IsDeploymentsNotFound(err error) bool {
	var apiErr *DeploymentsAPIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound
}
