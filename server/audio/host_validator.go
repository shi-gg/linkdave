package audio

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

func ValidateHost(urlStr string) (bool, error) {
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return false, fmt.Errorf("parse URL: %w", err)
	}

	switch parsedURL.Scheme {
	case "http":
		if !config.HTTPEnabled {
			return false, fmt.Errorf("http scheme is disabled")
		}
	case "https":
		if !config.HTTPSEnabled {
			return false, fmt.Errorf("https scheme is disabled")
		}
	default:
		return false, fmt.Errorf("unsupported URL scheme: %s", parsedURL.Scheme)
	}

	host := parsedURL.Hostname()
	if host == "" {
		return false, fmt.Errorf("empty hostname")
	}

	if config.PrivateIPAddressEnabled && config.PublicIPAddressEnabled {
		return true, nil
	}

	if strings.ToLower(host) == "localhost" {
		if !config.PrivateIPAddressEnabled {
			return false, fmt.Errorf("localhost not allowed")
		}
		return true, nil
	}

	ip := net.ParseIP(host)
	if ip == nil {
		ips, err := net.LookupIP(host)
		if err != nil {
			return false, fmt.Errorf("failed to resolve host: %w", err)
		}
		if len(ips) == 0 {
			return false, fmt.Errorf("no IPs resolved for host")
		}
		ip = ips[0]
	}

	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() {
		if !config.PrivateIPAddressEnabled {
			return false, fmt.Errorf("private IP address not allowed")
		}
		return true, nil
	}

	if !config.PublicIPAddressEnabled {
		return false, fmt.Errorf("public IP address not allowed")
	}

	return true, nil
}
