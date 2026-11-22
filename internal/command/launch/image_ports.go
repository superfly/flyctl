package launch

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

// Private network traffic bypasses fly-proxy, which counts these servers idle and stops them.
var databasePorts = map[int]string{
	3306:  "MySQL",
	4222:  "NATS",
	5432:  "Postgres",
	5672:  "RabbitMQ",
	6379:  "Redis",
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

	img, err := remote.Image(ref, remote.WithContext(ctx))
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
		if n, err := strconv.Atoi(port); err == nil {
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

// Fly's private network is IPv6, and mongod listens on IPv4 unless started with --ipv6.
var databasePortHints = map[int]string{
	27017: "mongod listens on IPv4 only unless started with --ipv6: launch with --command \"mongod --bind_ip_all --ipv6\", or set that command under [processes] in fly.toml and run `fly deploy`.",
}

func databaseImageNote(appName, image string, port int, ownCommand bool) string {
	note := fmt.Sprintf("%s has no public HTTP service: %s exposes %d, the %s port.\n"+
		"Reach it at %s.internal:%d, or from here with `fly proxy %d -a %s`. To add a public service: https://fly.io/docs/reference/configuration/#the-services-sections\n",
		appName, image, port, databasePorts[port], appName, port, port, appName)
	if hint, ok := databasePortHints[port]; ok && !ownCommand {
		note += hint + "\n"
	}

	return note
}
