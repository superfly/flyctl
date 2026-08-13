// Package mpg is a flyctl client for the public Managed Postgres API served by
// the Machines API (api.machines.dev) under /v1/postgres. That API proxies to
// ui-ex (FlyWeb.Api.Flaps.ManagedPostgresController) after a /v1 -> /flaps
// rewrite in flaps.
package mpg

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/superfly/fly-go/tokens"
	"github.com/superfly/flyctl/internal/config"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// Defaults for a Managed Postgres cluster.
const (
	DefaultUsername       = "fly-user"
	DefaultDatabase       = "fly-db"
	DefaultPort           = 5432
	DefaultPGMajorVersion = "16"
)

type contextKey struct{}

var clientContextKey = &contextKey{}

// Client is a client for the Managed Postgres API.
type Client interface {
	CreateCluster(ctx context.Context, input CreateClusterInput) (Cluster, error)
	GetCluster(ctx context.Context, id string) (Cluster, error)
	GetUserCredentials(ctx context.Context, id, username string) (UserCredentials, error)
}

// httpClient talks to the Managed Postgres API at the Machines API base URL.
type httpClient struct {
	client  *http.Client
	baseURL string
	tokens  *tokens.Tokens
}

var _ Client = (*httpClient)(nil)

// NewClient builds a client pointed at the configured Machines API base URL
// authenticating with the flaps token.
func NewClient(ctx context.Context) (Client, error) {
	cfg := config.FromContext(ctx)
	if cfg == nil {
		return nil, fmt.Errorf("config not found in context")
	}

	return &httpClient{
		client: &http.Client{
			Transport: otelhttp.NewTransport(http.DefaultTransport),
		},
		baseURL: strings.TrimSuffix(cfg.FlapsBaseURL, "/"),
		tokens:  cfg.Tokens,
	}, nil
}

// NewContextWithClient derives a Context that carries c.
func NewContextWithClient(ctx context.Context, c Client) context.Context {
	return context.WithValue(ctx, clientContextKey, c)
}

// ClientFromContext returns the Client ctx carries, or nil.
func ClientFromContext(ctx context.Context) Client {
	c, _ := ctx.Value(clientContextKey).(Client)

	return c
}

// Endpoint is a host/port pair for connecting to a cluster.
type Endpoint struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

// Endpoints groups a cluster's connection endpoints. Only populated once the
// cluster is ready.
type Endpoints struct {
	Primary struct {
		Direct Endpoint `json:"direct"`
		Pooler Endpoint `json:"pooler"`
	} `json:"primary"`
}

// OrganizationRef identifies the organization a cluster belongs to.
type OrganizationRef struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type AttachedApp struct {
	Name string `json:"name"`
}

// Cluster is the public projection of a Managed Postgres cluster returned by
// create/show. Fields use the public, unit-suffixed names from the API
// (disk_size_gb, memory_mb, cpus).
type Cluster struct {
	ID             string          `json:"id"`
	Name           string          `json:"name"`
	Status         string          `json:"status"`
	Region         string          `json:"region"`
	Plan           string          `json:"plan"`
	DiskSizeGB     int             `json:"disk_size_gb"`
	CPUs           int             `json:"cpus"`
	CPUKind        string          `json:"cpu_kind"`
	MemoryMB       int             `json:"memory_mb"`
	Replicas       int             `json:"replicas"`
	PGMajorVersion string          `json:"pg_major_version"`
	PostGISEnabled bool            `json:"postgis_enabled"`
	Endpoints      Endpoints       `json:"endpoints"`
	Organization   OrganizationRef `json:"organization"`
	CreatedAt      string          `json:"created_at"`
	AttachedApps   []AttachedApp   `json:"attached_apps"`
}

// ConnectionURI builds a libpq connection string for the cluster's default user
// and database via the pooler endpoint, using the given password.
func (c Cluster) ConnectionURI(password string) string {
	port := c.Endpoints.Primary.Pooler.Port
 	if port == 0 {
 		port = DefaultPort
 	}

	u := url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(DefaultUsername, password),
		Host:   fmt.Sprintf("%s:%d", c.Endpoints.Primary.Pooler.Host, port),
		Path:   "/" + DefaultDatabase,
	}

	return u.String()
}

type clusterEnvelope struct {
	Data Cluster `json:"data"`
}

// CreateClusterInput is the request body for POST /v1/postgres. org_slug is a
// body field (not a path segment) on this API.
type CreateClusterInput struct {
	OrgSlug        string `json:"org_slug"`
	Name           string `json:"name"`
	Region         string `json:"region"`
	Plan           string `json:"plan"`
	DiskSizeGB     int    `json:"disk_size_gb"`
	PGMajorVersion string `json:"pg_major_version"`
	PostGISEnabled bool   `json:"postgis_enabled"`
}

// UserCredentials is the response from GET
// /v1/postgres/:id/users/:username/credentials.
type UserCredentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type userCredentialsEnvelope struct {
	Data UserCredentials `json:"data"`
}

// CreateCluster provisions a new Managed Postgres cluster.
func (c *httpClient) CreateCluster(ctx context.Context, input CreateClusterInput) (Cluster, error) {
	var env clusterEnvelope
	if err := c.do(ctx, http.MethodPost, "/v1/postgres", input, &env, http.StatusCreated); err != nil {
		return Cluster{}, err
	}

	return env.Data, nil
}

// GetCluster fetches a cluster by its public id.
func (c *httpClient) GetCluster(ctx context.Context, id string) (Cluster, error) {
	var env clusterEnvelope

	path := "/v1/postgres/" + url.PathEscape(id)
	if err := c.do(ctx, http.MethodGet, path, nil, &env, http.StatusOK); err != nil {
		return Cluster{}, err
	}

	return env.Data, nil
}

// GetUserCredentials returns the username and current password for a named user.
func (c *httpClient) GetUserCredentials(ctx context.Context, id, username string) (UserCredentials, error) {
	var env userCredentialsEnvelope
	path := fmt.Sprintf("/v1/postgres/%s/users/%s/credentials", url.PathEscape(id), url.PathEscape(username))
	if err := c.do(ctx, http.MethodGet, path, nil, &env, http.StatusOK); err != nil {
		return UserCredentials{}, err
	}

	return env.Data, nil
}

// do issues a request and decodes a successful response into out. Any status
// other than wantStatus is returned as an error carrying the API's error
// message when present.
func (c *httpClient) do(ctx context.Context, method, path string, body, out any, wantStatus int) error {
	var reader io.Reader
	if body != nil {
		buf := &bytes.Buffer{}
		if err := json.NewEncoder(buf).Encode(body); err != nil {
			return fmt.Errorf("failed to encode request body: %w", err)
		}
		reader = buf
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", c.tokens.FlapsHeader())
	req.Header.Set("Content-Type", "application/json")

	res, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	respBody, err := io.ReadAll(res.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	if res.StatusCode != wantStatus {
		return fmt.Errorf("%s %s: unexpected status %d: %s", method, path, res.StatusCode, errorMessage(respBody))
	}

	if out != nil {
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
	}

	return nil
}

// errorMessage extracts the "error" field from an API error envelope, falling
// back to the raw body.
func errorMessage(body []byte) string {
	var env struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &env); err == nil && env.Error != "" {
		return env.Error
	}

	return string(body)
}
