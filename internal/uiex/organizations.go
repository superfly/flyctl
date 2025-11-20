package uiex

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/superfly/flyctl/internal/config"
)

type Organization struct {
	ID                       string        `json:"id"`
	InternalNumericID        uint64        `json:"internal_numeric_id"`
	Slug                     string        `json:"slug"`
	RawSlug                  string        `json:"raw_slug"`
	PaidPlan                 bool          `json:"paid_plan"`
	Personal                 bool          `json:"personal"`
	BillingStatus            BillingStatus `json:"billing_status"`
	ProvisionsBetaExtensions bool          `json:"provisions_beta_extensions"`
	Name                     string        `json:"name"`
	Billable                 bool          `json:"billable"`
	RemoteBuilderImage       string        `json:"remote_builder_image"`
}

type BillingStatus string

const (
	BillingStatusCurrent        BillingStatus = "CURRENT"
	BillingStatusDelinquent     BillingStatus = "DELINQUENT"
	BillingStatusPastDue        BillingStatus = "PAST_DUE"
	BillingStatusSourceRequired BillingStatus = "SOURCE_REQUIRED"
	BillingStatusSuspended      BillingStatus = "SUSPENDED"
	BillingStatusTrialActive    BillingStatus = "TRIAL_ACTIVE"
	BillingStatusTrialEnded     BillingStatus = "TRIAL_ENDED"
)

func (c *Client) ListOrganizations(ctx context.Context, admin bool) ([]Organization, error) {
	var err error

	var response struct {
		Organizations []Organization `json:"organizations"`
	}

	cfg := config.FromContext(ctx)
	url := fmt.Sprintf("%s/api/v1/organizations?admin=%t", c.baseUrl, admin)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return []Organization{}, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Add("Authorization", "Bearer "+cfg.Tokens.GraphQL())
	req.Header.Add("Content-Type", "application/json")

	res, err := c.httpClient.Do(req)
	if err != nil {
		return []Organization{}, err
	}
	defer res.Body.Close()

	switch res.StatusCode {
	case http.StatusOK:
		if err = json.NewDecoder(res.Body).Decode(&response); err != nil {
			return []Organization{}, fmt.Errorf("failed to decode response, please try again: %w", err)
		}
		return response.Organizations, nil
	case http.StatusNotFound:
		return []Organization{}, err
	default:
		return []Organization{}, err
	}
}

func (c *Client) GetOrganization(ctx context.Context, orgSlug string) (*Organization, error) {
	var err error

	var response struct {
		Organization Organization `json:"organization"`
	}

	cfg := config.FromContext(ctx)
	url := fmt.Sprintf("%s/api/v1/organizations/%s", c.baseUrl, orgSlug)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Add("Authorization", "Bearer "+cfg.Tokens.GraphQL())
	req.Header.Add("Content-Type", "application/json")

	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	switch res.StatusCode {
	case http.StatusOK:
		if err = json.NewDecoder(res.Body).Decode(&response); err != nil {
			return nil, fmt.Errorf("failed to decode response, please try again: %w", err)
		}
		return &response.Organization, nil
	case http.StatusNotFound:
		return nil, err
	default:
		return nil, err
	}
}
