package deploy

import (
	"context"
	"fmt"

	"github.com/docker/go-units"
	"github.com/samber/lo"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/prompt"
)

func (md *machineDeployment) provisionFirstDeploy(ctx context.Context, ipType string, org string) error {
	if !md.isFirstDeploy || md.restartOnly {
		return nil
	}
	if err := md.provisionIpsOnFirstDeploy(ctx, ipType, org); err != nil {
		fmt.Fprintf(md.io.ErrOut, "Failed to provision IP addresses. Use `fly ips` commands to remediate it. ERROR: %s", err)
	}
	if err := md.provisionVolumesOnFirstDeploy(ctx); err != nil {
		return fmt.Errorf("failed to provision seed volumes: %w", err)
	}
	return nil
}

func (md *machineDeployment) provisionIpsOnFirstDeploy(ctx context.Context, ipType string, org string) error {
	// Provision only if the app hasn't been deployed and have services defined
	if !md.isFirstDeploy || len(md.appConfig.AllServices()) == 0 || ipType == "none" {
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

	switch md.appConfig.DetermineIPType(ipType) {
	case "dedicated":
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

	case "shared":
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

	case "private":
		fmt.Fprintf(md.io.Out, "Provisioning ip address for %s\n", md.colorize.Bold(md.app.Name))
		v6Addr, err := md.apiClient.AllocateIPAddress(ctx, md.app.Name, "private_v6", org, nil, "")
		if err != nil {
			return fmt.Errorf("error allocating ipv6 after detecting first deploy and presence of services: %w", err)
		}
		fmt.Fprintf(md.io.Out, "  Private ipv6: %s\n", v6Addr.Address)
	}

	fmt.Fprintln(md.io.Out)
	return nil
}

func (md *machineDeployment) provisionVolumesOnFirstDeploy(ctx context.Context) error {
	// Provision only if the app hasn't been deployed and have mounts defined
	if !md.isFirstDeploy || len(md.appConfig.Mounts) == 0 {
		return nil
	}

	// md.setVolumes already queried for existent unattached volumes, do not create more
	existentVolumes := lo.MapValues(md.volumes, func(vs []fly.Volume, _ string) int {
		return len(vs)
	})

	// The logic here is to provision one volume per process group that needs it only on the primary region
	for _, groupName := range md.appConfig.ProcessNames() {
		groupConfig, err := md.appConfig.Flatten(groupName)
		if err != nil {
			return err
		}

		mConfig, err := md.appConfig.ToMachineConfig(groupName, nil)
		if err != nil {
			return err
		}
		guest := md.machineGuest
		if mConfig.Guest != nil {
			guest = mConfig.Guest
		}

		for _, m := range groupConfig.Mounts {
			if v := existentVolumes[m.Source]; v > 0 {
				existentVolumes[m.Source]--
				continue
			}

			var initialSize int
			switch {
			case m.InitialSize != "":
				// Ignore the error because invalid values are caught at config validation time
				initialSize, _ = helpers.ParseSize(m.InitialSize, units.FromHumanSize, units.GB)
			case md.volumeInitialSize > 0:
				initialSize = md.volumeInitialSize
			case guest != nil && guest.GPUKind != "":
				initialSize = DefaultGPUVolumeInitialSizeGB
			default:
				initialSize = DefaultVolumeInitialSizeGB
			}

			fmt.Fprintf(
				md.io.Out,
				"Creating a %d GB volume named '%s' for process group '%s'. "+
					"Use 'fly vol extend' to increase its size\n",
				initialSize, m.Source, groupName,
			)

			input := fly.CreateVolumeRequest{
				Name:                m.Source,
				Region:              groupConfig.PrimaryRegion,
				SizeGb:              fly.Pointer(initialSize),
				Encrypted:           fly.Pointer(true),
				ComputeRequirements: guest,
				ComputeImage:        md.img,
				SnapshotRetention:   m.SnapshotRetention,
			}

			vol, err := md.flapsClient.CreateVolume(ctx, input)
			if err != nil {
				return err
			}

			md.volumes[m.Source] = append(md.volumes[m.Source], *vol)
		}
	}
	return nil
}
