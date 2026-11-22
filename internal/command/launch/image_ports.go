package launch

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
)

// Private network traffic bypasses fly-proxy, which counts these servers idle and stops them.
var databasePorts = map[int]string{
	1433:  "SQL Server",
	1883:  "MQTT",
	3306:  "MySQL",
	4222:  "NATS",
	5432:  "Postgres",
	5672:  "RabbitMQ",
	6379:  "Redis",
	9092:  "Kafka",
	9200:  "Elasticsearch",
	11211: "Memcached",
	27017: "MongoDB",
}

const imageInspectTimeout = 10 * time.Second

var imageExposedPorts = func(ctx context.Context, image string) ([]int, error) {
	ref, err := name.ParseReference(image)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, imageInspectTimeout)
	defer cancel()

	img, err := remote.Image(ref, remote.WithContext(ctx), remote.WithRetryBackoff(remote.Backoff{Steps: 1}))
	if err != nil {
		return nil, err
	}
	cfg, err := img.ConfigFile()
	if err != nil {
		return nil, err
	}

	var ports []int
	for exposed := range cfg.Config.ExposedPorts {
		port, proto, _ := strings.Cut(exposed, "/")
		if proto != "" && proto != "tcp" {
			continue
		}
		if n, err := strconv.Atoi(port); err == nil && n >= 1 && n <= 65535 {
			ports = append(ports, n)
		}
	}
	slices.Sort(ports)

	return ports, nil
}

func imageServicePort(ports []int, defaultPort int) (servicePort, databasePort int) {
	for _, p := range ports {
		if _, ok := databasePorts[p]; ok {
			return 0, p
		}
	}
	if len(ports) == 1 {
		return ports[0], 0
	}

	return defaultPort, 0
}

func imageLookupWarning(image string, port int, err error) string {
	return fmt.Sprintf("Warning: couldn't read the exposed ports of %s (%s); using internal_port %d. Set --internal-port to choose the port.\n",
		image, imageLookupReason(image, err), port)
}

func imageLookupReason(image string, err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Sprintf("timed out after %s", imageInspectTimeout)
	}
	var terr *transport.Error
	if !errors.As(err, &terr) {
		return err.Error()
	}
	rateLimited := terr.StatusCode == http.StatusTooManyRequests || slices.ContainsFunc(terr.Errors, func(d transport.Diagnostic) bool {
		return d.Code == transport.TooManyRequestsErrorCode
	})
	if rateLimited {
		registry := "registry"
		if ref, err := name.ParseReference(image); err == nil {
			registry = ref.Context().RegistryStr()
		}
		if registry == name.DefaultRegistry {
			registry = "Docker Hub"
		}

		return registry + " rate limit reached"
	}
	if len(terr.Errors) > 0 && terr.Errors[0].Message != "" {
		return terr.Errors[0].Message
	}

	return terr.Error()
}

// Fly's private network is IPv6, and mongod listens on IPv4 unless started with --ipv6.
var databasePortHints = map[int]string{
	27017: "mongod listens on IPv4 only unless started with --ipv6: launch with --command \"mongod --bind_ip_all --ipv6\", or set that command under [processes] in fly.toml and run `fly deploy`.",
}

func databaseImageNote(appName, image string, port int, ownCommand, flycast bool) string {
	note := fmt.Sprintf("%s has no public HTTP service: %s exposes %d, the %s port.\n"+
		"From other apps in this org: %s.internal:%d. From this computer: `fly proxy %d -a %s`.\n"+
		"To add a public service, add it to fly.toml (https://docs.fly.io/reference/configuration#the-services-sections), then run `fly deploy`, `fly ips allocate-v6` and `fly ips allocate-v4 --shared`.\n",
		appName, image, port, databasePorts[port], appName, port, port, appName)
	if flycast {
		note += fmt.Sprintf("%s.flycast has no address: Flycast needs a [[services]] entry for port %d, then `fly deploy` and `fly ips allocate-v6 --private`.\n", appName, port)
	}
	if hint, ok := databasePortHints[port]; ok && !ownCommand {
		note += hint + "\n"
	}

	return note
}
