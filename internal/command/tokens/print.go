package tokens

import (
	"context"
	"fmt"
	"io"
	"reflect"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/macaroon"
	"github.com/superfly/macaroon/auth"
	"github.com/superfly/macaroon/flyio"
	"github.com/superfly/macaroon/resset"
)

func newPrint() *cobra.Command {
	const (
		short = "Parse token from stdin and print caveat details"
		long  = "Reads a Fly.io API token string (potentially multiple concatenated tokens) from standard input, parses it, prints the details of each caveat found, and finally prints the total caveat count."
		usage = "print"
	)

	cmd := command.New(usage, short, long, runPrint)
	return cmd
}

func runPrint(ctx context.Context) error {
	ios := iostreams.FromContext(ctx)

	inputBytes, err := io.ReadAll(ios.In)
	if err != nil {
		return fmt.Errorf("failed to read from stdin: %w", err)
	}

	if len(inputBytes) == 0 {
		return fmt.Errorf("no token provided via stdin")
	}

	inputString := string(inputBytes)
	if len(inputString) > 0 && inputString[len(inputString)-1] == '\n' {
		inputString = inputString[:len(inputString)-1]
	}

	parsedTokens, err := macaroon.Parse(inputString)
	if err != nil {
		return fmt.Errorf("failed to parse token string: %w", err)
	}

	for i, mtok := range parsedTokens {
		m, err := macaroon.Decode(mtok)
		if err != nil {
			fmt.Fprintf(ios.ErrOut, "Warning: failed to decode macaroon part %d: %v\n", i, err)
			continue
		}

		// Categorize and print token type based on Location
		location := m.Location
		var tokenType string
		if location == flyio.LocationAuthentication {
			tokenType = "Authentication Token"
		} else if strings.HasPrefix(location, "https://api.fly.io/") {
			tokenType = "Permissions Token"
		} else {
			tokenType = "External Token"
		}
		fmt.Fprintf(ios.Out, "%s (%s)\n", tokenType, location)

		allCaveats := macaroon.GetCaveats[macaroon.Caveat](&m.UnsafeCaveats)

		fmt.Fprintf(ios.Out, "  %d caveats:\n", len(allCaveats))
		for _, cav := range allCaveats {
			printCaveat(ios, cav)
		}
		fmt.Fprintln(ios.Out, "---") // Separator between macaroons

	}

	return nil
}

// printCaveat attempts to identify the type of a caveat and print its details.
func printCaveat(ios *iostreams.IOStreams, cav macaroon.Caveat) {
	prin := func(format string, args ...interface{}) {
		fmt.Fprintf(ios.Out, format, args...)
	}

	switch c := cav.(type) {
	case *macaroon.Caveat3P:
		if c.Location == flyio.LocationAuthentication {
			prin("  Caveat: Require Authentication Macaroon (%s)\n", c.Location)
		} else {
			prin("  Caveat: Require Macaroon From (%s)\n", c.Location)
		}
	case *macaroon.BindToParentToken:
		prin("  Caveat: Bound To Parent Token\n")
	case *macaroon.ValidityWindow:
		now := time.Now()
		notBefore := time.Unix(c.NotBefore, 0)
		notAfter := time.Unix(c.NotAfter, 0)

		if now.After(notBefore) && now.Before(notAfter) {
			remaining := notAfter.Sub(now)
			prin("  Caveat: Validity Window - Valid For %s\n", formatDuration(remaining))
		} else {
			prin("  Caveat: Validity Window - NotBefore: %s, NotAfter: %s (INVALID)\n",
				notBefore.Format(time.RFC3339),
				notAfter.Format(time.RFC3339),
			)
		}

	case *flyio.Apps:
		prin("  Caveat: Apps - %s\n", c.Apps)
	case *flyio.Volumes:
		prin("  Caveat: Volumes - %s\n", c.Volumes)
	case *flyio.Machines:
		prin("  Caveat: Machines - %s\n", c.Machines)
	case *flyio.Clusters:
		prin("  Caveat: Clusters - %s\n", c.Clusters)
	case *flyio.FeatureSet:
		prin("  Caveat: Org Features - %s\n", c.Features)
	case *flyio.AppFeatureSet:
		prin("  Caveat: App Features - %s\n", c.Features)
	case *flyio.MachineFeatureSet:
		prin("  Caveat: Machine Features - %s\n", c.Features)
	case *flyio.FromMachine:
		prin("  Caveat: From Machine ID: %s\n", c.ID)
	case *flyio.StorageObjects:
		prin("  Caveat: Storage Objects - %s\n", c.Prefixes)
	case *flyio.AllowedRoles:
		prin("  Caveat: Allowed Roles - %s\n", flyio.Role(*c))
	case *flyio.IsMember:
		prin("  Caveat: Is Member (Requires at least Member role)\n")
	case *flyio.IsUser:
		prin("  Attestation: Fly.io User ID: %d (Deprecated Caveat Version)\n", c.ID)
	case *flyio.FlySrc:
		prin("  Caveat: Fly Source - Org: %s, App: %s, Instance: %s\n", c.Organization, c.App, c.Instance)
	case *flyio.Organization:
		prin("  Caveat: Organization - ID: %d, Mask: %s\n", c.ID, formatAccessMask(c.Mask))
	case *flyio.Mutations:
		prin("  Caveat: Mutations - Allowed: %v\n", c.Mutations)
	case *flyio.Commands:
		prin("  Caveat: Commands - %d allowed commands:\n", len(*c))
		for _, cmd := range *c {
			prin("    - Args: %v, Exact: %t\n", cmd.Args, cmd.Exact)
		}
	case *resset.IfPresent:
		// TODO: Need a way to print CaveatSet details
		prin("  Caveat: IfPresent - Ifs: %T, Else: %s\n", c.Ifs, formatAccessMask(c.Else))
	// Auth package caveats
	case *auth.ConfineOrganization:
		prin("  Caveat: Require Authentication from Fly.io Org ID: %d\n", c.ID)
	case *auth.ConfineUser:
		prin("  Caveat: Require Authentication from Fly.io User ID: %d\n", c.ID)
	case *auth.ConfineGoogleHD:
		prin("  Caveat: Require Authentication from Google Domain: %s\n", string(*c))
	case *auth.ConfineGitHubOrg:
		prin("  Caveat: Require Authentication from GitHub Org ID: %d\n", uint64(*c))
	case *auth.MaxValidity:
		prin("  Caveat: Max Discharge Validity: %s\n", (time.Duration(*c) * time.Second).String())
	// Attestations (not restrictions)
	case *auth.FlyioUserID:
		prin("  Attestation: Fly.io User ID: %d\n", uint64(*c))
	case *auth.GitHubUserID:
		prin("  Attestation: GitHub User ID: %d\n", uint64(*c))
	case *auth.GoogleUserID:
		prin("  Attestation: Google User ID: %s\n", c)
	default:
		// Use reflection for unknown types for basic info
		typeName := reflect.TypeOf(cav).String()
		prin("  Caveat: Unknown Type (%s) - Value: %+v\n", typeName, cav)
	}
}

// formatAccessMask converts a resset.Action mask (uint16) into a human-readable string.
func formatAccessMask(mask resset.Action) string {
	if mask == resset.ActionAll {
		return "all-access"
	}
	if mask == resset.ActionNone {
		return "no-access"
	}

	var parts []string

	if mask&resset.ActionRead != 0 {
		parts = append(parts, "read")
	}
	if mask&resset.ActionWrite != 0 {
		parts = append(parts, "write")
	}
	if mask&resset.ActionCreate != 0 {
		parts = append(parts, "create")
	}
	if mask&resset.ActionDelete != 0 {
		parts = append(parts, "destroy")
	}
	if mask&resset.ActionControl != 0 {
		parts = append(parts, "control")
	}

	if len(parts) == 0 {
		return fmt.Sprintf("unknown-access (mask: %d)", mask)
	}

	return strings.Join(parts, ", ")
}

// formatDuration converts a duration into a human-readable string (days, weeks, hours, minutes).
func formatDuration(d time.Duration) string {
	const (
		day  = 24 * time.Hour
		week = 7 * day
	)

	var (
		units int
		unit  string
	)

	switch {
	case d < time.Minute:
		seconds := int(d / time.Second)
		if seconds <= 0 {
			return "less than a minute"
		}
		units = seconds
		unit = "second"
	case d < time.Hour:
		units = int(d / time.Minute)
		unit = "minute"
	case d < day:
		units = int(d / time.Hour)
		unit = "hour"
	case d < week:
		units = int(d / day)
		unit = "day"
	default: // d >= week
		units = int(d / week)
		unit = "week"
	}

	if units != 1 {
		unit += "s"
	}

	return fmt.Sprintf("%d %s", units, unit)
}
