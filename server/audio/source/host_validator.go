package source

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

func ValidateHost(urlStr string) (string, error) {
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return "", fmt.Errorf("parse URL: %w", err)
	}

	switch parsedURL.Scheme {
	case "http":
		if !cfg.HTTPEnabled {
			return "", fmt.Errorf("http scheme is disabled")
		}
	case "https":
		if !cfg.HTTPSEnabled {
			return "", fmt.Errorf("https scheme is disabled")
		}
	default:
		return "", fmt.Errorf("unsupported URL scheme: %s", parsedURL.Scheme)
	}

	host := parsedURL.Hostname()
	if host == "" {
		return "", fmt.Errorf("empty hostname")
	}

	if cfg.PrivateIPAddressEnabled && cfg.PublicIPAddressEnabled {
		return host, nil
	}

	if strings.ToLower(host) == "localhost" {
		if !cfg.PrivateIPAddressEnabled {
			return "", fmt.Errorf("localhost not allowed")
		}
		return "127.0.0.1", nil
	}

	ip := net.ParseIP(host)
	if ip == nil {
		ips, err := net.LookupIP(host)
		if err != nil {
			return "", fmt.Errorf("failed to resolve host: %w", err)
		}
		if len(ips) == 0 {
			return "", fmt.Errorf("no IPs resolved for host")
		}
		ip = ips[0]
	}

	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
		if !cfg.PrivateIPAddressEnabled {
			return "", fmt.Errorf("private IP address not allowed")
		}
		return ip.String(), nil
	}

	if !cfg.PublicIPAddressEnabled {
		return "", fmt.Errorf("public IP address not allowed")
	}

	return ip.String(), nil
}
