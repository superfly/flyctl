package utils

import (
	"context"
	"fmt"

	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/flyutil"
)

// AliasedOrganizationSlug resolves organization slug the aliased slug
// using GraphQL.
//
// Example:
//
//	Input:  "jon-phenow"
//	Output: "personal" (if "jon-phenow" is an alias for "personal")
//
// GraphQL Query:
//
//	query {
//	    organization(slug: "jon-phenow"){
//	        slug
//	    }
//	}
//
// Response:
//
//	{
//	    "data": {
//	        "organization": {
//	            "slug": "personal"
//	        }
//	    }
//	}
func AliasedOrganizationSlug(ctx context.Context, inputSlug string) (string, error) {
	client := flyutil.ClientFromContext(ctx)
	genqClient := client.GenqClient()

	// Query the GraphQL API to resolve the organization slug
	resp, err := gql.GetOrganization(ctx, genqClient, inputSlug)
	if err != nil {
		return "", fmt.Errorf("failed to resolve organization slug %q: %w", inputSlug, err)
	}

	// Return the canonical slug from the API response
	return resp.Organization.Slug, nil
}

// ResolveOrganizationSlug resolves organization slug aliases to the canonical slug
// using GraphQL. This handles cases where users use aliases that map to different
// canonical organization slugs.
//
// Example:
//
//	Input:  "personal"
//	Output: "jon-phenow" (if "personal" is an alias for "jon-phenow")
//
// GraphQL Query:
//
//	query {
//	    organization(slug: "personal"){
//	        rawSlug
//	    }
//	}
//
// Response:
//
//	{
//	    "data": {
//	        "organization": {
//	            "rawSlug": "jon-phenow"
//	        }
//	    }
//	}
func ResolveOrganizationSlug(ctx context.Context, inputSlug string) (string, error) {
	client := flyutil.ClientFromContext(ctx)
	genqClient := client.GenqClient()

	// Query the GraphQL API to resolve the organization slug
	resp, err := gql.GetOrganization(ctx, genqClient, inputSlug)
	if err != nil {
		return "", fmt.Errorf("failed to resolve organization slug %q: %w", inputSlug, err)
	}

	// Return the canonical slug from the API response
	return resp.Organization.RawSlug, nil
}
