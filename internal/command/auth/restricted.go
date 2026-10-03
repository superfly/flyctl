package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/macaroon"
	"github.com/superfly/macaroon/flyio"
	"github.com/superfly/macaroon/resset"
)

const (
	restrictedTokenName     = "flyctl restricted login"
	restrictedTokenProfile  = "deploy_organization"
	restrictedDefaultExpiry = 30 * 24 * time.Hour
)

var restrictedFlags = []string{
	"org",
	"mask",
	"expiry",
}

type restrictedPreset struct {
	label string
	mask  string
}

var restrictedPresets = []restrictedPreset{
	{
		label: "No deletes (rwcC)",
		mask:  "rwcC",
	},
	{
		label: "Read-only (r)",
		mask:  "r",
	},
}

func restrictedFlagsWithoutRestricted(ctx context.Context) error {
	if flag.GetBool(ctx, "restricted") {
		return nil
	}

	for _, name := range restrictedFlags {
		if flag.IsSpecified(ctx, name) {
			return fmt.Errorf("--%s requires --restricted", name)
		}
	}

	return nil
}

func restrictToken(ctx context.Context, userToken string) (string, error) {
	client := flyutil.NewClientFromOptions(ctx, fly.ClientOptions{
		AccessToken: userToken,
	})

	orgs, err := client.GetOrganizations(ctx)
	if err != nil {
		return "", fmt.Errorf("failed retrieving organizations: %w", err)
	}

	selected, err := selectRestrictedOrgs(ctx, orgs)
	if err != nil {
		return "", err
	}

	mask, err := restrictedMask(ctx)
	if err != nil {
		return "", err
	}

	expiry := flag.GetDuration(ctx, "expiry")
	if expiry <= 0 {
		return "", errors.New("--expiry must be positive")
	}

	var toks [][]byte
	for _, org := range selected {
		orgToks, err := mintRestrictedToken(ctx, client, org, mask, expiry)
		if err != nil {
			return "", err
		}
		toks = append(toks, orgToks...)
	}

	printRestrictedSummary(ctx, selected, mask, expiry)

	return macaroon.ToAuthorizationHeader(toks...), nil
}

func selectRestrictedOrgs(
	ctx context.Context,
	orgs []fly.Organization,
) ([]fly.Organization, error) {
	if slugs := flag.GetStringSlice(ctx, "org"); len(slugs) > 0 {
		return orgsBySlug(orgs, slugs)
	}

	options := make([]string, 0, len(orgs))
	for _, org := range orgs {
		options = append(options, fmt.Sprintf("%s (%s)", org.Name, org.Slug))
	}

	var indices []int
	err := prompt.MultiSelect(ctx,
		&indices,
		"Which organizations should this login have access to?",
		nil,
		options...,
	)
	switch {
	case prompt.IsNonInteractive(err):
		return nil, requiredWhenNonInteractive("org")
	case err != nil:
		return nil, err
	case len(indices) == 0:
		return nil, errors.New("no organizations selected")
	}

	selected := make([]fly.Organization, 0, len(indices))
	for _, i := range indices {
		selected = append(selected, orgs[i])
	}

	return selected, nil
}

func orgsBySlug(
	orgs []fly.Organization,
	slugs []string,
) ([]fly.Organization, error) {
	bySlug := make(map[string]fly.Organization, len(orgs))
	for _, org := range orgs {
		bySlug[org.Slug] = org
		bySlug[org.RawSlug] = org
	}

	selected := make([]fly.Organization, 0, len(slugs))
	for _, slug := range slugs {
		org, ok := bySlug[slug]
		if !ok {
			return nil, fmt.Errorf("organization %s not found", slug)
		}
		selected = append(selected, org)
	}

	return selected, nil
}

func restrictedMask(ctx context.Context) (resset.Action, error) {
	if mask := flag.GetString(ctx, "mask"); mask != "" {
		return parseMask(mask)
	}

	options := make([]string, 0, len(restrictedPresets))
	for _, p := range restrictedPresets {
		options = append(options, p.label)
	}

	var index int
	err := prompt.Select(ctx,
		&index,
		"What should this login be allowed to do?",
		options[0],
		options...,
	)
	switch {
	case prompt.IsNonInteractive(err):
		index = 0
	case err != nil:
		return 0, err
	}

	return parseMask(restrictedPresets[index].mask)
}

func parseMask(mask string) (resset.Action, error) {
	if strings.Trim(mask, "rwcdC") != "" {
		return 0, fmt.Errorf(
			"invalid mask %q: use only r, w, c, d and C",
			mask,
		)
	}

	return resset.ActionFromString(mask), nil
}

func mintRestrictedToken(
	ctx context.Context,
	client flyutil.Client,
	org fly.Organization,
	mask resset.Action,
	expiry time.Duration,
) ([][]byte, error) {
	resp, err := gql.CreateLimitedAccessToken(ctx,
		client.GenqClient(),
		restrictedTokenName,
		org.ID,
		restrictedTokenProfile,
		&gql.LimitedAccessTokenOptions{},
		expiry.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed creating token for %s: %w", org.Slug, err)
	}

	perm, diss, err := macaroon.ParsePermissionAndDischargeTokens(
		resp.CreateLimitedAccessToken.LimitedAccessToken.TokenHeader,
		flyio.LocationPermission,
	)
	if err != nil {
		return nil, fmt.Errorf("failed parsing token for %s: %w", org.Slug, err)
	}

	mac, err := macaroon.Decode(perm)
	if err != nil {
		return nil, fmt.Errorf("failed decoding token for %s: %w", org.Slug, err)
	}

	orgID, err := flyio.OrganizationScope(&mac.UnsafeCaveats)
	if err != nil {
		return nil, fmt.Errorf("token for %s has no org scope: %w", org.Slug, err)
	}

	err = mac.Add(&flyio.Organization{
		ID:   orgID,
		Mask: mask,
	})
	if err != nil {
		return nil, fmt.Errorf("failed attenuating token for %s: %w", org.Slug, err)
	}

	perm, err = mac.Encode()
	if err != nil {
		return nil, fmt.Errorf("failed encoding token for %s: %w", org.Slug, err)
	}

	return append([][]byte{perm}, diss...), nil
}

func printRestrictedSummary(
	ctx context.Context,
	orgs []fly.Organization,
	mask resset.Action,
	expiry time.Duration,
) {
	var slugs strings.Builder
	for i, org := range orgs {
		if i > 0 {
			slugs.WriteString(", ")
		}
		slugs.WriteString(org.Slug)
	}

	io := iostreams.FromContext(ctx)
	fmt.Fprintf(io.Out,
		"restricted to %s with permissions %s, expiring in %s\n",
		slugs.String(),
		mask,
		expiry,
	)
}

func revokeUserToken(ctx context.Context, userToken string) error {
	client := flyutil.NewClientFromOptions(ctx, fly.ClientOptions{
		AccessToken: userToken,
	})

	resp, err := gql.LogOut(ctx, client.GenqClient())
	if err != nil {
		return err
	}
	if !resp.LogOut.Ok {
		return errors.New("logout was not accepted by the API")
	}

	return nil
}
