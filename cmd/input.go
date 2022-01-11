package cmd

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/superfly/flyctl/api"
)

func isInterrupt(err error) bool {
	return err != nil && err.Error() == "interrupt"
}

func confirm(message string) bool {
	confirm := false
	prompt := &survey.Confirm{
		Message: message,
	}
	err := survey.AskOne(prompt, &confirm)
	checkErr(err)

	return confirm
}

func confirmOverwrite(filename string) bool {
	confirm := false
	prompt := &survey.Confirm{
		Message: fmt.Sprintf(`Overwrite "%s"?`, filename),
	}
	err := survey.AskOne(prompt, &confirm)
	checkErr(err)

	return confirm
}

func selectOrganization(ctx context.Context, client *api.Client, slug string, typeFilter *api.OrganizationType) (*api.Organization, error) {
	orgs, err := client.GetOrganizations(ctx, typeFilter)
	if err != nil {
		return nil, err
	}

	if slug != "" {
		for _, org := range orgs {
			if org.Slug == slug {
				return &org, nil
			}
		}

		return nil, fmt.Errorf(`organization "%s" not found`, slug)
	}

	if len(orgs) == 1 && orgs[0].Type == "PERSONAL" {
		fmt.Printf("Automatically selected %s organization: %s\n", strings.ToLower(orgs[0].Type), orgs[0].Name)
		return &orgs[0], nil
	}

	sort.Slice(orgs, func(i, j int) bool { return orgs[i].Type < orgs[j].Type })

	options := []string{}

	for _, org := range orgs {
		options = append(options, fmt.Sprintf("%s (%s)", org.Name, org.Slug))
	}

	selectedOrg := 0
	prompt := &survey.Select{
		Message:  "Select organization:",
		Options:  options,
		PageSize: 15,
	}
	if err := survey.AskOne(prompt, &selectedOrg); err != nil {
		return nil, err
	}

	return &orgs[selectedOrg], nil
}

func selectWireGuardPeer(ctx context.Context, client *api.Client, slug string) (string, error) {
	peers, err := client.GetWireGuardPeers(ctx, slug)
	if err != nil {
		return "", err
	}

	if len(peers) < 1 {
		return "", fmt.Errorf(`Organization "%s" does not have any wireguard peers`, slug)
	}

	var options []string
	for _, peer := range peers {
		options = append(options, peer.Name)
	}

	selectedPeer := 0
	prompt := &survey.Select{
		Message:  "Select peer:",
		Options:  options,
		PageSize: 30,
	}
	if err := survey.AskOne(prompt, &selectedPeer); err != nil {
		return "", err
	}

	return peers[selectedPeer].Name, nil
}

func selectRegion(ctx context.Context, client *api.Client, regionCode string) (*api.Region, error) {
	regions, requestRegion, err := client.PlatformRegions(ctx)
	if err != nil {
		return nil, err
	}

	if regionCode != "" {
		for _, region := range regions {
			if region.Code == regionCode {
				return &region, nil
			}
		}

		return nil, fmt.Errorf(`region "%s" not found`, regionCode)
	}

	options := []string{}

	for _, region := range regions {
		options = append(options, fmt.Sprintf("%s (%s)", region.Code, region.Name))
	}

	selectedRegion := 0
	prompt := &survey.Select{
		Message:  "Select region:",
		Options:  options,
		PageSize: 15,
	}

	if requestRegion != nil {
		prompt.Default = fmt.Sprintf("%s (%s)", requestRegion.Code, requestRegion.Name)
	}

	if err := survey.AskOne(prompt, &selectedRegion); err != nil {
		return nil, err
	}

	return &regions[selectedRegion], nil
}

func selectVMSize(ctx context.Context, client *api.Client, vmSizeName string) (*api.VMSize, error) {
	vmSizes, err := client.PlatformVMSizes(ctx)
	if err != nil {
		return nil, err
	}

	if vmSizeName != "" {
		for _, vmSize := range vmSizes {
			if vmSize.Name == vmSizeName {
				return &vmSize, nil
			}
		}

		return nil, fmt.Errorf(`vm size "%s" not found`, vmSizeName)
	}

	options := []string{}

	for _, vmSize := range vmSizes {
		options = append(options, fmt.Sprintf("%s - %d", vmSize.Name, vmSize.MemoryMB))
	}

	selectedVMSize := 0
	prompt := &survey.Select{
		Message:  "Select VM size:",
		Options:  options,
		PageSize: 15,
	}
	if err := survey.AskOne(prompt, &selectedVMSize); err != nil {
		return nil, err
	}

	return &vmSizes[selectedVMSize], nil
}

func inputAppName(defaultName string, autoGenerate bool) (name string, err error) {
	message := "App Name"

	if autoGenerate {
		message += " (leave blank to use an auto-generated name)"
	}

	message += ":"

	prompt := &survey.Input{
		Message: message,
		Default: defaultName,
	}

	if err := survey.AskOne(prompt, &name); err != nil {
		return name, err
	}

	return name, nil
}

func initialClusterSizeInput(defaultVal int) (int, error) {
	var count int
	prompt := &survey.Input{
		Message: "Specify the initial cluster size:",
		Default: strconv.Itoa(defaultVal),
	}
	if err := survey.AskOne(prompt, &count); err != nil {
		return 0, err
	}

	return count, nil
}

func volumeSizeInput(defaultVal int) (int, error) {
	var volumeSize int
	prompt := &survey.Input{
		Message: "Volume size (GB):",
		Default: strconv.Itoa(defaultVal),
	}
	if err := survey.AskOne(prompt, &volumeSize); err != nil {
		return 0, err
	}

	return volumeSize, nil
}
