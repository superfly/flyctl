// Package command implements helpers useful for when building cobra commands.
// This source file contains logic common to `fly machine run` and `fly console`
package command

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/dustin/go-humanize"
	"github.com/pkg/errors"
	"github.com/samber/lo"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/iostreams"
	"golang.org/x/exp/slices"

	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/build/imgsrc"
	"github.com/superfly/flyctl/internal/cmdutil"
	"github.com/superfly/flyctl/internal/env"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/state"
)

func DetermineImage(ctx context.Context, appName string, imageOrPath string) (img *imgsrc.DeploymentImage, err error) {
	var (
		client = flyutil.ClientFromContext(ctx)
		io     = iostreams.FromContext(ctx)
		cfg    = appconfig.ConfigFromContext(ctx)
	)

	daemonType := imgsrc.NewDockerDaemonType(!flag.GetBool(ctx, "build-remote-only"), !flag.GetBool(ctx, "build-local-only"), env.IsCI(), flag.GetBool(ctx, "build-depot"), flag.GetBool(ctx, "build-nixpacks"))
	resolver := imgsrc.NewResolver(daemonType, client, appName, io, flag.GetWireguard(ctx), false)

	// build if relative or absolute path
	if strings.HasPrefix(imageOrPath, ".") || strings.HasPrefix(imageOrPath, "/") {
		opts := imgsrc.ImageOptions{
			AppName:              appName,
			WorkingDir:           path.Join(state.WorkingDirectory(ctx)),
			Publish:              !flag.GetBuildOnly(ctx),
			ImageLabel:           flag.GetString(ctx, "image-label"),
			Target:               flag.GetString(ctx, "build-target"),
			NoCache:              flag.GetBool(ctx, "no-build-cache"),
			BuildpacksDockerHost: flag.GetString(ctx, flag.BuildpacksDockerHost),
			BuildpacksVolumes:    flag.GetStringSlice(ctx, flag.BuildpacksVolume),
		}

		dockerfilePath := cfg.Dockerfile()

		// dockerfile passed through flags takes precedence over the one set in config
		if flag.GetString(ctx, "dockerfile") != "" {
			dockerfilePath = flag.GetString(ctx, "dockerfile")
		}

		if dockerfilePath != "" {
			dockerfilePath, err := filepath.Abs(dockerfilePath)
			if err != nil {
				return nil, err
			}
			opts.DockerfilePath = dockerfilePath
		}

		extraArgs, err := cmdutil.ParseKVStringsToMap(flag.GetStringArray(ctx, "build-arg"))
		if err != nil {
			return nil, errors.Wrap(err, "invalid build-arg")
		}
		opts.BuildArgs = extraArgs

		if cfg != nil && cfg.Experimental != nil {
			opts.UseZstd = cfg.Experimental.UseZstd
		}

		// use-zstd passed through flags takes precedence over the one set in config
		if flag.IsSpecified(ctx, "use-zstd") {
			opts.UseZstd = flag.GetBool(ctx, "use-zstd")
		}

		img, err = resolver.BuildImage(ctx, io, opts)
		if err != nil {
			return nil, err
		}
		if img == nil {
			return nil, errors.New("could not find an image to deploy")
		}
	} else {
		opts := imgsrc.RefOptions{
			AppName:    appName,
			WorkingDir: state.WorkingDirectory(ctx),
			Publish:    !flag.GetBool(ctx, "build-only"),
			ImageRef:   imageOrPath,
			ImageLabel: flag.GetString(ctx, "image-label"),
		}

		img, err = resolver.ResolveReference(ctx, io, opts)
		if err != nil {
			return nil, err
		}
	}

	if img == nil {
		return nil, errors.New("could not find an image to deploy")
	}

	fmt.Fprintf(io.Out, "Image: %s\n", img.String())
	fmt.Fprintf(io.Out, "Image size: %s\n\n", humanize.Bytes(uint64(img.Size)))

	return img, nil
}

func DetermineServices(ctx context.Context, services []fly.MachineService) ([]fly.MachineService, error) {
	svcKey := func(internalPort int, protocol string) string {
		return fmt.Sprintf("%d/%s", internalPort, protocol)
	}
	servicesRef := lo.Map(services, func(s fly.MachineService, _ int) *fly.MachineService { return &s })
	servicesMap := lo.KeyBy(servicesRef, func(s *fly.MachineService) string {
		return svcKey(s.InternalPort, s.Protocol)
	})

	for _, p := range flag.GetStringSlice(ctx, "port") {
		internalPort, proto, edgePort, edgeStartPort, edgeEndPort, handlers, err := parsePortFlag(p)
		if err != nil {
			return nil, err
		}

		// Look for existing services or append a new one
		svc, ok := servicesMap[svcKey(internalPort, proto)]
		if !ok {
			svc = &fly.MachineService{
				InternalPort: internalPort,
				Protocol:     proto,
			}
			servicesRef = append(servicesRef, svc)
			servicesMap[svcKey(internalPort, proto)] = svc
		}

		// A dash handler removes the service: --port 5432/tcp:-
		if slices.Equal(handlers, []string{"-"}) {
			svc.Ports = nil
			continue
		}

		// Look for existing ports and update them
		found := false
		for idx := range svc.Ports {
			svcPort := &svc.Ports[idx]
			if svcPort.Port != nil && edgePort != nil && *(svcPort.Port) == *edgePort {
				found = true
				svcPort.Handlers = handlers
			}
			if svcPort.StartPort != nil && edgeStartPort != nil && *(svcPort.StartPort) == *edgeStartPort {
				found = true
				svcPort.Handlers = handlers
				svcPort.EndPort = edgeEndPort
			}
		}
		// Or append new port definition
		if !found {
			svc.Ports = append(svc.Ports, fly.MachinePort{
				Port:      edgePort,
				StartPort: edgeStartPort,
				EndPort:   edgeEndPort,
				Handlers:  handlers,
			})
		}
	}

	// Remove any service without exposed ports
	services = lo.FilterMap(servicesRef, func(s *fly.MachineService, _ int) (fly.MachineService, bool) {
		if s != nil && len(s.Ports) > 0 {
			return *s, true
		}
		return fly.MachineService{}, false
	})

	return services, nil
}

func parsePortFlag(str string) (internalPort int, protocol string, port, startPort, endPort *int, handlers []string, err error) {
	protocol = "tcp"
	splittedPortsProto := strings.Split(str, "/")
	if len(splittedPortsProto) == 2 {
		splittedProtoHandlers := strings.Split(splittedPortsProto[1], ":")
		protocol = splittedProtoHandlers[0]
		handlers = append(handlers, splittedProtoHandlers[1:]...)
	} else if len(splittedPortsProto) > 2 {
		err = errors.New("port must be at most two elements (ports/protocol:handler)")
		return
	}

	port, startPort, endPort, internalPort, err = parsePorts(splittedPortsProto[0])
	if internalPort == 0 {
		switch {
		case port != nil:
			internalPort = *port
		case startPort != nil:
			internalPort = *startPort
		}
	}
	return
}

func parsePorts(input string) (port, start_port, end_port *int, internal_port int, err error) {
	split := strings.Split(input, ":")
	if len(split) == 1 {
		var external_port int
		external_port, err = strconv.Atoi(split[0])
		if err != nil {
			err = errors.Wrap(err, "invalid port")
			return
		}

		port = fly.IntPointer(external_port)
	} else if len(split) == 2 {
		internal_port, err = strconv.Atoi(split[1])
		if err != nil {
			err = errors.Wrap(err, "invalid machine (internal) port")
			return
		}

		external_split := strings.Split(split[0], "-")
		if len(external_split) == 1 {
			var external_port int
			external_port, err = strconv.Atoi(external_split[0])
			if err != nil {
				err = errors.Wrap(err, "invalid external port")
				return
			}

			port = fly.IntPointer(external_port)
		} else if len(external_split) == 2 {
			var start int
			start, err = strconv.Atoi(external_split[0])
			if err != nil {
				err = errors.Wrap(err, "invalid start port for port range")
				return
			}

			start_port = fly.IntPointer(start)

			var end int
			end, err = strconv.Atoi(external_split[0])
			if err != nil {
				err = errors.Wrap(err, "invalid end port for port range")
				return
			}

			end_port = fly.IntPointer(end)
		} else {
			err = errors.New("external port must be at most 2 elements (port, or range start-end)")
		}
	} else {
		err = errors.New("port definition must be at most 2 elements (external:internal)")
	}

	return
}

// FilesFromCommand checks the specified flags for files and returns a list of fly.File to be used
// in the machine configuration.
func FilesFromCommand(ctx context.Context) ([]*fly.File, error) {
	machineFiles := []*fly.File{}

	localFiles, err := parseFiles(ctx, "file-local", func(value string, file *fly.File) error {
		content, err := os.ReadFile(value)
		if err != nil {
			return fmt.Errorf("could not read file %s: %w", value, err)
		}
		rawValue := base64.StdEncoding.EncodeToString(content)
		file.RawValue = &rawValue
		return nil
	})
	if err != nil {
		return machineFiles, fmt.Errorf("failed to read file-local: %w", err)
	}
	machineFiles = append(machineFiles, localFiles...)

	literalFiles, err := parseFiles(ctx, "file-literal", func(value string, file *fly.File) error {
		encodedValue := base64.StdEncoding.EncodeToString([]byte(value))
		file.RawValue = &encodedValue
		return nil
	})
	if err != nil {
		return machineFiles, fmt.Errorf("failed to read file-literal: %w", err)
	}
	machineFiles = append(machineFiles, literalFiles...)

	secretFiles, err := parseFiles(ctx, "file-secret", func(value string, file *fly.File) error {
		file.SecretName = &value
		return nil
	})
	if err != nil {
		return machineFiles, fmt.Errorf("failed to read file-secret: %w", err)
	}
	machineFiles = append(machineFiles, secretFiles...)

	return machineFiles, nil
}

func parseFiles(ctx context.Context, flagName string, cb func(value string, file *fly.File) error) ([]*fly.File, error) {
	flagFiles := flag.GetStringArray(ctx, flagName)
	machineFiles := make([]*fly.File, 0, len(flagFiles))

	for _, f := range flagFiles {
		guestPath, fileRef, ok := strings.Cut(f, "=")
		file := fly.File{
			GuestPath: guestPath,
		}
		switch {
		case !ok:
			return nil, fmt.Errorf("invalid %s argument %s", flagName, f)
		case !filepath.IsAbs(guestPath):
			return nil, fmt.Errorf("guest path, %s, must be absolute", guestPath)
		case fileRef == "":
			// empty value is allowed to remove file from machine
		default:
			if err := cb(fileRef, &file); err != nil {
				return nil, err
			}
		}

		machineFiles = append(machineFiles, &file)
	}

	return machineFiles, nil
}

func DetermineMounts(ctx context.Context, mounts []fly.MachineMount, region string) ([]fly.MachineMount, error) {
	unattachedVolumes := make(map[string][]fly.Volume)

	pathIndex := make(map[string]int)
	for idx, m := range mounts {
		pathIndex[m.Path] = idx
	}

	for _, v := range flag.GetStringSlice(ctx, "volume") {
		splittedIDDestOpts := strings.Split(v, ":")
		if len(splittedIDDestOpts) < 2 {
			return nil, fmt.Errorf("Can't infer volume and mount path from '%s'", v)
		}
		volID := splittedIDDestOpts[0]
		mountPath := splittedIDDestOpts[1]

		if !strings.HasPrefix(volID, "vol_") {
			volName := volID

			// Load app volumes the first time
			if len(unattachedVolumes) == 0 {
				var err error
				unattachedVolumes, err = getUnattachedVolumes(ctx, region)
				if err != nil {
					return nil, err
				}
			}

			if len(unattachedVolumes[volName]) == 0 {
				return nil, fmt.Errorf("not enough unattached volumes for '%s'", volName)
			}
			volID = unattachedVolumes[volName][0].ID
			unattachedVolumes[volName] = unattachedVolumes[volName][1:]
		}

		if idx, found := pathIndex[mountPath]; found {
			mounts[idx].Volume = volID
		} else {
			mounts = append(mounts, fly.MachineMount{
				Volume: volID,
				Path:   mountPath,
			})
		}
	}
	return mounts, nil
}

func getUnattachedVolumes(ctx context.Context, regionCode string) (map[string][]fly.Volume, error) {
	apiclient := flyutil.ClientFromContext(ctx)
	flapsClient := flapsutil.ClientFromContext(ctx)

	if regionCode == "" {
		region, err := apiclient.GetNearestRegion(ctx)
		if err != nil {
			return nil, err
		}
		regionCode = region.Code
	}

	volumes, err := flapsClient.GetVolumes(ctx)
	if err != nil {
		return nil, fmt.Errorf("Error fetching application volumes: %w", err)
	}

	unattached := lo.Filter(volumes, func(v fly.Volume, _ int) bool {
		return !v.IsAttached() && (regionCode == v.Region)
	})
	if len(unattached) == 0 {
		return nil, fmt.Errorf("No unattached volumes in region '%s'", regionCode)
	}

	unattachedMap := lo.GroupBy(unattached, func(v fly.Volume) string { return v.Name })
	return unattachedMap, nil
}
