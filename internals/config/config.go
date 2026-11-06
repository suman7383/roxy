package config

import (
	"errors"
	"net"
	"net/url"
	"os"
	"strconv"
	"time"

	"go.yaml.in/yaml/v3"
)

const (
	defaultListenAddr          = ":8080"
	defaultRequestTimeout      = 5 * time.Second
	defaultHealthCheckInterval = 10 * time.Second
	defaultHealthCheckTimeout  = 2 * time.Second
)

type Config struct {
	ListenAddr string   `yaml:"listen_addr"`
	Backends   []string `yaml:"backends"`

	RequestTimeout      time.Duration `yaml:"request_timeout"`
	HealthCheckInterval time.Duration `yaml:"health_check_interval"`
	HealthCheckTimeout  time.Duration `yaml:"health_check_timeout"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	c := &Config{}

	// Load the defaults
	c.LoadDefaults()

	if err := yaml.Unmarshal(data, c); err != nil {
		return nil, err
	}

	if err := c.validate(); err != nil {
		return nil, err
	}

	return c, nil
}

func (c *Config) validate() error {
	// Validate the listen_addr is valid with port,
	// use net.SplitHostPort
	_, port, err := net.SplitHostPort(c.ListenAddr)
	if err != nil {
		return errors.New("listen_addr is invalid")
	}

	// port should be valid
	if portInt, err := strconv.Atoi(port); err != nil || portInt < 1 || portInt > 65535 {
		return errors.New("listen_addr port is invalid")
	}

	if len(c.Backends) == 0 {
		return errors.New("backends is required")
	}

	// Check for valid URL format, scheme is http or https, host is not empty, port is not empty
	for _, backend := range c.Backends {
		parsedURL, err := url.Parse(backend)
		if err != nil ||
			(parsedURL.Scheme != "http" && parsedURL.Scheme != "https") ||
			parsedURL.Host == "" {
			return errors.New("backend URL is invalid")
		}
	}

	if c.RequestTimeout <= 0 {
		return errors.New("request_timeout is required")
	}
	if c.HealthCheckInterval <= 0 {
		return errors.New("health_check_interval is required")
	}
	if c.HealthCheckTimeout <= 0 {
		return errors.New("health_check_timeout is required")
	}
	return nil
}

func (c *Config) LoadDefaults() {
	c.ListenAddr = defaultListenAddr
	c.RequestTimeout = defaultRequestTimeout
	c.HealthCheckInterval = defaultHealthCheckInterval
	c.HealthCheckTimeout = defaultHealthCheckTimeout
}
