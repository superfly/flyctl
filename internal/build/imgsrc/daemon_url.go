package imgsrc

import (
	"fmt"
	"net"
	"net/url"
)

func builderAPIURL(daemonHost, path string) (string, error) {
	parsed, err := url.Parse(daemonHost)
	if err != nil {
		return "", err
	}

	hostname := parsed.Hostname()
	if hostname == "" {
		return "", fmt.Errorf("failed to parse daemon host %q: missing hostname", daemonHost)
	}

	parsed.Host = net.JoinHostPort(hostname, "8080")
	parsed.Scheme = "http"
	parsed.Path = path

	return parsed.String(), nil
}
