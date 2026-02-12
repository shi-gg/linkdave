package audio

import (
	"os"
	"strconv"
)

type SourceConfig struct {
	HTTPEnabled             bool
	HTTPSEnabled            bool
	PublicIPAddressEnabled  bool
	PrivateIPAddressEnabled bool
}

var config SourceConfig

func init() {
	config = SourceConfig{
		HTTPEnabled:             getEnvBool("LINKDAVE_SOURCE_HTTP_ENABLED", false),
		HTTPSEnabled:            getEnvBool("LINKDAVE_SOURCE_HTTPS_ENABLED", false),
		PublicIPAddressEnabled:  getEnvBool("LINKDAVE_SOURCE_IP_ADDRESS_PUBLIC_ENABLED", false),
		PrivateIPAddressEnabled: getEnvBool("LINKDAVE_SOURCE_IP_ADDRESS_PRIVATE_ENABLED", false),
	}
}

func GetConfig() SourceConfig {
	return config
}

func getEnvBool(key string, defaultValue bool) bool {
	val := os.Getenv(key)
	if val == "" {
		return defaultValue
	}
	b, err := strconv.ParseBool(val)
	if err != nil {
		return defaultValue
	}
	return b
}
