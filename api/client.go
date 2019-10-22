package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
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
}

func NewClient(accessToken string) (*Client, error) {
	if accessToken == "" {
		return nil, errors.New("No api access token available. Please login")
	}

	httpClient, _ := newHTTPClient()

	url := fmt.Sprintf("%s/api/v2/graphql", baseURL)

	client := graphql.NewClient(url, graphql.WithHTTPClient(httpClient))
	return &Client{httpClient, client, accessToken}, nil
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

func (c *Client) GetCurrentUser() (*User, error) {
	query := `
		query {
			currentUser {
				email
			}
		}
	`

	req := c.NewRequest(query)

	data, err := c.Run(req)
	if err != nil {
		return nil, err
	}

	return &data.CurrentUser, nil
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

type getLogsResponse struct {
	Data []struct {
		Id         string
		Attributes LogEntry
	}
	Meta struct {
		NextToken string `json:"next_token"`
	}
}

func (c *Client) GetAppLogs(appName string, nextToken string, region string, instanceId string) ([]LogEntry, string, error) {

	data := url.Values{}
	data.Set("next_token", nextToken)
	if instanceId != "" {
		data.Set("instance", instanceId)
	}
	if region != "" {
		data.Set("region", region)
	}

	url := fmt.Sprintf("%s/api/v1/apps/%s/logs?%s", baseURL, appName, data.Encode())

	req, err := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.accessToken))

	var result getLogsResponse

	entries := []LogEntry{}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return entries, "", err
	}

	if resp.StatusCode != 200 {
		return entries, "", ErrorFromResp(resp)
	}

	defer resp.Body.Close()

	json.NewDecoder(resp.Body).Decode(&result)

	for _, d := range result.Data {
		entries = append(entries, d.Attributes)
	}

	return entries, result.Meta.NextToken, nil
}

func (client *Client) GetOrganizations() ([]Organization, error) {
	q := `
		{
			organizations {
				nodes {
					id
					slug
					name
					type
				}
			}
		}
	`

	req := client.NewRequest(q)

	data, err := client.Run(req)
	if err != nil {
		return []Organization{}, err
	}

	return data.Organizations.Nodes, nil
}

func (client *Client) DeployImage(input DeployImageInput) (*Release, error) {
	query := `
			mutation($input: DeployImageInput!) {
				deployImage(input: $input) {
					release {
						id
						version
						reason
						description
						user {
							id
							email
							name
						}
						createdAt
					}
				}
			}
		`

	req := client.NewRequest(query)

	req.Var("input", input)

	data, err := client.Run(req)
	if err != nil {
		return nil, err
	}

	return &data.DeployImage.Release, nil
}

func (c *Client) GetApps() ([]App, error) {
	query := `
		query {
			apps(type: "container") {
				nodes {
					id
					name
					deployed
					hostname
					organization {
						slug
					}
					deploymentStatus {
						createdAt
					}
				}
			}
		}
		`

	req := c.NewRequest(query)

	data, err := c.Run(req)
	if err != nil {
		return nil, err
	}

	return data.Apps.Nodes, nil
}

func (c *Client) GetApp(appName string) (*App, error) {
	query := `
		query ($appName: String!) {
			app(name: $appName) {
				id
				name
				hostname
				deployed
				status
				version
				appUrl
				organization {
					slug
				}
				tasks {
					id
					name
					services {
						protocol
						port
						internalPort
						filters
					}
				}
				ipAddresses {
					nodes {
						id
						address
						type
					}
				}
			}
		}
	`

	req := c.NewRequest(query)
	req.Var("appName", appName)

	data, err := c.Run(req)
	if err != nil {
		return nil, err
	}

	return &data.App, nil
}

func (c *Client) GetAppReleases(appName string, limit int) ([]Release, error) {
	query := `
		query ($appName: String!, $limit: Int!) {
			app(name: $appName) {
				releases(first: $limit) {
					nodes {
						id
						version
						reason
						description
						reason
						status
						stable
						user {
							id
							email
							name
						}	
						createdAt
					}
				}
			}
		}
	`

	req := c.NewRequest(query)

	req.Var("appName", appName)
	req.Var("limit", limit)

	data, err := c.Run(req)
	if err != nil {
		return nil, err
	}

	return data.App.Releases.Nodes, nil
}

func (c *Client) GetAppCurrentRelease(appName string) (*Release, error) {
	query := `
		query ($appName: String!) {
			app(name: $appName) {
				currentRelease {
					version
					stable
					inProgress
					description
					status
					reason
					revertedTo
					deploymentStrategy
					createdAt
					user {
						email
					}
					deployment {
						status
						description
						tasks {
							name
							placed
							healthy
							desired
							canaries
							promoted
							unhealthy
							progressDeadline
						}
					}
				}
			}
		}
	`

	req := c.NewRequest(query)

	req.Var("appName", appName)

	data, err := c.Run(req)
	if err != nil {
		return nil, err
	}

	return data.App.CurrentRelease, nil
}

func (c *Client) GetAppReleaseVersion(appName string, version int) (*Release, error) {
	query := `
		query ($appName: String!, $version: Int!) {
			app(name: $appName) {
				release(version: $version) {
					version
					stable
					inProgress
					description
					status
					reason
					revertedTo
					deploymentStrategy
					createdAt
					user {
						email
					}
					deployment {
						status
						description
						tasks {
							name
							placed
							healthy
							desired
							canaries
							promoted
							unhealthy
							progressDeadline
						}
					}
				}
			}
		}
	`

	req := c.NewRequest(query)

	req.Var("appName", appName)
	req.Var("version", version)

	data, err := c.Run(req)
	if err != nil {
		return nil, err
	}

	return data.App.Release, nil
}

func (c *Client) SetSecrets(appName string, secrets map[string]string) (*Release, error) {
	query := `
		mutation($input: SetSecretsInput!) {
			setSecrets(input: $input) {
				release {
					id
					version
					reason
					description
					user {
						id
						email
						name
					}
					createdAt
				}
			}
		}
	`

	input := SetSecretsInput{AppID: appName}
	for k, v := range secrets {
		input.Secrets = append(input.Secrets, SetSecretsInputSecret{Key: k, Value: v})
	}

	req := c.NewRequest(query)

	req.Var("input", input)

	data, err := c.Run(req)
	if err != nil {
		return nil, err
	}

	return &data.SetSecrets.Release, nil
}

func (c *Client) UnsetSecrets(appName string, keys []string) (*Release, error) {
	query := `
		mutation($input: UnsetSecretsInput!) {
			unsetSecrets(input: $input) {
				release {
					id
					version
					reason
					description
					user {
						id
						email
						name
					}
					createdAt
				}
			}
		}
	`

	req := c.NewRequest(query)

	req.Var("input", UnsetSecretsInput{AppID: appName, Keys: keys})

	data, err := c.Run(req)
	if err != nil {
		return nil, err
	}

	return &data.UnsetSecrets.Release, nil
}

func (c *Client) GetAppSecrets(appName string) ([]Secret, error) {
	query := `
		query ($appName: String!) {
			app(name: $appName) {
				secrets {
					name
					digest
					createdAt
				}
			}
		}
	`

	req := c.NewRequest(query)

	req.Var("appName", appName)

	data, err := c.Run(req)
	if err != nil {
		return nil, err
	}

	return data.App.Secrets, nil
}

func (c *Client) CreateApp(name string, orgId string) (*App, error) {
	query := `
		mutation($input: CreateAppInput!) {
			createApp(input: $input) {
				app {
					id
					name
					organization {
						slug
					}
				}
			}
		}
	`

	req := c.NewRequest(query)

	req.Var("input", CreateAppInput{
		Name:           name,
		Runtime:        "FIRECRACKER",
		OrganizationID: orgId,
	})

	data, err := c.Run(req)
	if err != nil {
		return nil, err
	}

	return &data.CreateApp.App, nil
}

func (c *Client) GetAppStatus(appName string, showComplete bool) (*App, error) {
	query := `
		query($appName: String!, $showComplete: Boolean!) {
			app(name: $appName) {
				id
				name
				deployed
				status
				hostname
				version
				appUrl
				organization {
					slug
				}
				tasks {
					id
					name
					status
					servicesSummary
					allocations(complete: $showComplete) {
						id
						version
						latestVersion
						status
						desiredStatus
						region
						createdAt
					}
				}
				currentRelease {
					version
					stable
					inProgress
					description
					status
					reason
					revertedTo
					deploymentStrategy
					user {
						email
					}
					createdAt
					deployment {
						status
						description
						tasks {
							name
							placed
							healthy
							desired
							canaries
							promoted
							unhealthy
							progressDeadline
						}
					}
				}
			}
		}
	`

	req := c.NewRequest(query)
	req.Var("appName", appName)
	req.Var("showComplete", showComplete)

	data, err := c.Run(req)
	if err != nil {
		return nil, err
	}

	return &data.App, nil
}

func (c *Client) CreateSignedUrls(appId string, filename string) (getUrl string, putUrl string, err error) {
	query := `
		mutation($appId: ID!, $filename: String!) {
			createSignedUrl(appId: $appId, filename: $filename) {
				getUrl
				putUrl
			}
		}
	`

	req := c.NewRequest(query)

	req.Var("appId", appId)
	req.Var("filename", filename)

	data, err := c.Run(req)
	if err != nil {
		return "", "", err
	}

	return data.CreateSignedUrl.GetUrl, data.CreateSignedUrl.PutUrl, nil
}

func (c *Client) CreateBuild(appId string, sourceUrl, sourceType string) (*Build, error) {
	query := `
		mutation($appId: ID!, $sourceUrl: String!, $sourceType: UrlSource!) {
			createBuild(appId: $appId, sourceUrl: $sourceUrl, sourceType: $sourceType) {
				build {
					id
					inProgress
					status
					user {
						id
						name
						email
					}
					createdAt
					updatedAt
				}
			}
		}
	`

	req := c.NewRequest(query)

	req.Var("appId", appId)
	req.Var("sourceUrl", sourceUrl)
	req.Var("sourceType", sourceType)

	data, err := c.Run(req)
	if err != nil {
		return nil, err
	}

	return &data.CreateBuild.Build, nil
}

func (c *Client) ListBuilds(appName string) ([]Build, error) {
	query := `
		query($appName: String!) {
			app(name: $appName) {
				builds {
					nodes {
						id
						inProgress
						status
						user {
							id
							name
							email
						}
						createdAt
						updatedAt
					}
				}
			}
		}
	`

	req := c.NewRequest(query)

	req.Var("appName", appName)

	data, err := c.Run(req)
	if err != nil {
		return nil, err
	}

	return data.App.Builds.Nodes, nil
}

func (c *Client) GetBuild(buildId string) (*Build, error) {
	query := `
		query($id: ID!) {
			build: node(id: $id) {
				id
				__typename
				... on Build {
					inProgress
					status
					logs
					user {
						id
						name
						email
					}
					createdAt
					updatedAt
				}
			}
		}
	`

	req := c.NewRequest(query)

	req.Var("id", buildId)

	data, err := c.Run(req)
	if err != nil {
		return nil, err
	}

	return &data.Build, nil
}

func (client *Client) DeleteApp(appName string) error {
	query := `
			mutation($appId: ID!) {
				deleteApp(appId: $appId) {
					organization {
						id
					}
				}
			}
		`

	req := client.NewRequest(query)

	req.Var("appId", appName)

	_, err := client.Run(req)
	return err
}

func (c *Client) GetAppChanges(appName string) ([]AppChange, error) {
	query := `
		query($appName: String!) {
			app(name: $appName) {
				changes {
					nodes {
						id
						description
						status
						actor {
							type: __typename
						}
						user {
							id
							email
						}
						createdAt
						updatedAt
					}
				}
			}
		}
	`

	req := c.NewRequest(query)

	req.Var("appName", appName)

	data, err := c.Run(req)
	if err != nil {
		return nil, err
	}

	return data.App.Changes.Nodes, nil
}

func (client *Client) GetDatabases() ([]Database, error) {
	q := `
		{
			databases {
				nodes {
					id
					backendId
					key
					name
					engine
					organization {
						id
						name
						slug
					}
					createdAt
				}
			}
		}
	`

	req := client.NewRequest(q)

	data, err := client.Run(req)
	if err != nil {
		return []Database{}, err
	}

	return data.Databases.Nodes, nil
}

func (client *Client) GetDatabase(key string) (*Database, error) {
	q := `
		query($key: String!) {
			database(key: $key) {
				id
				backendId
				key
				name
				engine
				vmUrl
				publicUrl
				organization {
					id
					name
					slug
				}
				createdAt
			}
		}
	`

	req := client.NewRequest(q)

	req.Var("key", key)

	data, err := client.Run(req)
	if err != nil {
		return nil, err
	}

	return &data.Database, nil
}

func (c *Client) CreateDatabase(orgID string, name string, engine string) (*Database, error) {
	query := `
		mutation($input: CreateDatabaseInput!) {
			createDatabase(input: $input) {
				database {
					id
					backendId
					name
					engine
					vmUrl
					publicUrl
					organization {
						id
						name
						slug
					}
					createdAt
				}
			}
		}
	`

	req := c.NewRequest(query)

	req.Var("input", map[string]string{
		"organizationId": orgID,
		"engine":         strings.ToUpper(engine),
		"name":           name,
	})

	data, err := c.Run(req)
	if err != nil {
		return nil, err
	}

	return &data.CreateDatabase.Database, nil
}

func (c *Client) DestroyDatabase(databaseId string) (*Organization, error) {
	query := `
		mutation($databaseId: ID!) {
			destroyDatabase(databaseId: $databaseId) {
				organization {
					id
					slug
					name
				}
			}
		}
	`

	req := c.NewRequest(query)

	req.Var("databaseId", databaseId)

	data, err := c.Run(req)
	if err != nil {
		return nil, err
	}

	return &data.DestroyDatabase.Organization, nil
}

func (c *Client) GetAppCertificates(appName string) ([]AppCertificate, error) {
	query := `
		query($appName: String!) {
			app(name: $appName) {
				certificates {
					nodes {
						createdAt
						hostname
						clientStatus
					}
				}
			}
		}
	`

	req := c.NewRequest(query)

	req.Var("appName", appName)

	data, err := c.Run(req)
	if err != nil {
		return nil, err
	}

	return data.App.Certificates.Nodes, nil
}

func (c *Client) GetAppCertificate(appName string, hostname string) (*AppCertificate, error) {
	query := `
		query($appName: String!, $hostname: String!) {
			app(name: $appName) {
				certificate(hostname: $hostname) {
					acmeDnsConfigured
					certificateAuthority
					createdAt
					dnsProvider
					dnsValidationInstructions
					dnsValidationHostname
					dnsValidationTarget
					hostname
					id
					source
					clientStatus
					issued {
						nodes {
							type
							expiresAt
						}
					}
				}
			}
		}
	`

	req := c.NewRequest(query)

	req.Var("appName", appName)
	req.Var("hostname", hostname)

	data, err := c.Run(req)
	if err != nil {
		return nil, err
	}

	return &data.App.Certificate, nil
}

func (c *Client) CheckAppCertificate(appName string, hostname string) (*AppCertificate, error) {
	query := `
		query($appName: String!, $hostname: String!) {
			app(name: $appName) {
				certificate(hostname: $hostname) {
					acmeDnsConfigured
					certificateAuthority
					createdAt
					dnsProvider
					dnsValidationInstructions
					dnsValidationHostname
					dnsValidationTarget
					hostname
					id
					source
					clientStatus
					issued {
						nodes {
							type
							expiresAt
						}
					}
				}
			}
		}
	`

	req := c.NewRequest(query)

	req.Var("appName", appName)
	req.Var("hostname", hostname)

	data, err := c.Run(req)
	if err != nil {
		return nil, err
	}

	return &data.App.Certificate, nil
}

func (c *Client) AddCertificate(appName string, hostname string) (*AppCertificate, error) {
	query := `
		mutation($appId: ID!, $hostname: String!) {
			addCertificate(appId: $appId, hostname: $hostname) {
				certificate {
					acmeDnsConfigured
					certificateAuthority
					certificateRequestedAt
					dnsProvider
					dnsValidationTarget
					hostname
					id
					source
				}
			}
		}
	`

	req := c.NewRequest(query)

	req.Var("appId", appName)
	req.Var("hostname", hostname)

	data, err := c.Run(req)
	if err != nil {
		return nil, err
	}

	return &data.AddCertificate.Certificate, nil
}

func (c *Client) DeleteCertificate(appName string, hostname string) (*DeleteCertificatePayload, error) {
	query := `
		mutation($appId: ID!, $hostname: String!) {
			deleteCertificate(appId: $appId, hostname: $hostname) {
				app {
					name
				}
				certificate {
					hostname
					id
				}
			}
		}
	`

	req := c.NewRequest(query)

	req.Var("appId", appName)
	req.Var("hostname", hostname)

	data, err := c.Run(req)
	if err != nil {
		return nil, err
	}

	return &data.DeleteCertificate, nil
}

func (c *Client) GetAppServices(appName string) ([]Service, error) {
	query := `
		query($appName: String!) {
			app(name: $appName) {
				services {
					protocol
					port
					internalPort
					handlers
					description
					checks {
						name
						interval
						timeout
						type
						httpPath
						httpMethod
						httpHeaders {
							name
							value
						}
						httpProtocol
						httpTlsSkipVerify
					}
				}
			}
		}
	`

	req := c.NewRequest(query)

	req.Var("appName", appName)

	data, err := c.Run(req)
	if err != nil {
		return nil, err
	}

	return data.App.Services, nil
}
