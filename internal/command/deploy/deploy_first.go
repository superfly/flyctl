package deploy

import (
	"context"
	"fmt"

	"github.com/samber/lo"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/internal/prompt"
)

func (md *machineDeployment) provisionFirstDeploy(ctx context.Context) error {
	if !md.isFirstDeploy || md.restartOnly {
		return nil
	}
	if err := md.provisionIpsOnFirstDeploy(ctx); err != nil {
		fmt.Fprintf(md.io.ErrOut, "Failed to provision IP addresses, use `fly ips` commands to remmediate it. ERROR: %s", err)
	}
	if err := md.provisionVolumesOnFirstDeploy(ctx); err != nil {
		return fmt.Errorf("failed to provision seed volumes: %w", err)
	}
	return nil
}

func (md *machineDeployment) provisionIpsOnFirstDeploy(ctx context.Context) error {
	// Provision only if the app hasn't been deployed and have services defined
	if !md.isFirstDeploy || len(md.appConfig.AllServices()) == 0 {
		return nil
	}

	// Do not touch IPs if there are already allocated
	ipAddrs, err := md.apiClient.GetIPAddresses(ctx, md.app.Name)
	if err != nil {
		return fmt.Errorf("error detecting ip addresses allocated to %s app: %w", md.app.Name, err)
	}
	if len(ipAddrs) > 0 {
		return nil
	}

	switch md.appConfig.HasNonHttpAndHttpsStandardServices() {
	case true:
		hasUdpService := md.appConfig.HasUdpService()

		ipStuffStr := "a dedicated ipv4 address"
		if !hasUdpService {
			ipStuffStr = "dedicated ipv4 and ipv6 addresses"
		}

		confirmDedicatedIp, err := prompt.Confirmf(ctx, "Would you like to allocate %s now?", ipStuffStr)
		if confirmDedicatedIp && err == nil {
			v4Dedicated, err := md.apiClient.AllocateIPAddress(ctx, md.app.Name, "v4", "", nil, "")
			if err != nil {
				return err
			}
			fmt.Fprintf(md.io.Out, "Allocated dedicated ipv4: %s\n", v4Dedicated.Address)

			if !hasUdpService {
				v6Dedicated, err := md.apiClient.AllocateIPAddress(ctx, md.app.Name, "v6", "", nil, "")
				if err != nil {
					return err
				}
				fmt.Fprintf(md.io.Out, "Allocated dedicated ipv6: %s\n", v6Dedicated.Address)
			}
		}

	case false:
		fmt.Fprintf(md.io.Out, "Provisioning ips for %s\n", md.colorize.Bold(md.app.Name))
		v6Addr, err := md.apiClient.AllocateIPAddress(ctx, md.app.Name, "v6", "", nil, "")
		if err != nil {
			return fmt.Errorf("error allocating ipv6 after detecting first deploy and presence of services: %w", err)
		}
		fmt.Fprintf(md.io.Out, "  Dedicated ipv6: %s\n", v6Addr.Address)

		v4Shared, err := md.apiClient.AllocateSharedIPAddress(ctx, md.app.Name)
		if err != nil {
			return fmt.Errorf("error allocating shared ipv4 after detecting first deploy and presence of services: %w", err)
		}
		fmt.Fprintf(md.io.Out, "  Shared ipv4: %s\n", v4Shared)
		fmt.Fprintf(md.io.Out, "  Add a dedicated ipv4 with: fly ips allocate-v4\n")
	}

	return nil
}

func (md *machineDeployment) provisionVolumesOnFirstDeploy(ctx context.Context) error {
	// Provision only if the app hasn't been deployed and have mounts defined
	if !md.isFirstDeploy || len(md.appConfig.Mounts) == 0 {
		return nil
	}

	// md.setVolumes already queried for existent unattached volumes, do not create more
	existentVolumes := lo.MapValues(md.volumes, func(vs []api.Volume, _ string) int {
		return len(vs)
	})

	// The logic here is to provision one volume per process group that needs it only on the primary region
	for _, groupName := range md.appConfig.ProcessNames() {
		groupConfig, err := md.appConfig.Flatten(groupName)
		if err != nil {
			return err
		}

		for _, m := range groupConfig.Mounts {
			if v := existentVolumes[m.Source]; v > 0 {
				existentVolumes[m.Source]--
				continue
			}

			fmt.Fprintf(md.io.Out, "Creating 1GB volume '%s' for process group '%s'. Use 'fly vol extend' to increase its size\n", m.Source, groupName)

			input := api.CreateVolumeInput{
				AppID:     md.app.ID,
				Name:      m.Source,
				Region:    groupConfig.PrimaryRegion,
				SizeGb:    1,
				Encrypted: true,
			}

			vol, err := md.apiClient.CreateVolume(ctx, input)
			if err != nil {
				return fmt.Errorf("failed creating volume: %w", err)
			}

			md.volumes[m.Source] = append(md.volumes[m.Source], *vol)
		}
	}
	return nil
}
