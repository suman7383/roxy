package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func validConfig() *Config {
	return &Config{
		ListenAddr:          ":8080",
		Backends:            []string{"http://localhost:8081", "https://example.com"},
		RequestTimeout:      5 * time.Second,
		HealthCheckInterval: 10 * time.Second,
		HealthCheckTimeout:  2 * time.Second,
	}
}

func writeConfigFile(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.yaml")

	err := os.WriteFile(path, []byte(content), 0644)
	if err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	return path
}

// Load tests

func TestLoad_UsesDefaults(t *testing.T) {
	path := writeConfigFile(t, `
backends:
  - "http://localhost:8081"
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}

	if cfg.ListenAddr != defaultListenAddr {
		t.Errorf("ListenAddr = %q, want %q", cfg.ListenAddr, defaultListenAddr)
	}

	if cfg.RequestTimeout != defaultRequestTimeout {
		t.Errorf("RequestTimeout = %v, want %v", cfg.RequestTimeout, defaultRequestTimeout)
	}

	if cfg.HealthCheckInterval != defaultHealthCheckInterval {
		t.Errorf("HealthCheckInterval = %v, want %v", cfg.HealthCheckInterval, defaultHealthCheckInterval)
	}

	if cfg.HealthCheckTimeout != defaultHealthCheckTimeout {
		t.Errorf("HealthCheckTimeout = %v, want %v", cfg.HealthCheckTimeout, defaultHealthCheckTimeout)
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	path := writeConfigFile(t, `
backends: [
`)

	cfg, err := Load(path)

	if err == nil {
		t.Fatal("Load() expected an error for invalid YAML")
	}

	if cfg != nil {
		t.Errorf("Load() returned config = %v, want nil", cfg)
	}
}

func TestLoad_InvalidConfig(t *testing.T) {
	path := writeConfigFile(t, `
backends: []
`)

	cfg, err := Load(path)

	if err == nil {
		t.Fatal("Load() expected an error for invalid config")
	}

	if cfg != nil {
		t.Errorf("Load() returned config = %v, want nil", cfg)
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.yaml")

	cfg, err := Load(path)

	if err == nil {
		t.Fatal("Load() expected an error for missing file")
	}

	if cfg != nil {
		t.Errorf("Load() returned config = %v, want nil", cfg)
	}
}

// Config validation tests

func TestConfigValidate_ValidConfig(t *testing.T) {
	cfg := validConfig()

	if err := cfg.validate(); err != nil {
		t.Fatalf("validate() returned unexpected error: %v", err)
	}
}

func TestConfigValidate_EmptyListenAddr(t *testing.T) {
	cfg := validConfig()
	cfg.ListenAddr = ""

	if err := cfg.validate(); err == nil {
		t.Fatal("validate() expected error for empty listen address")
	}
}

func TestConfigValidate_EmptyBackends(t *testing.T) {
	cfg := validConfig()
	cfg.Backends = nil

	if err := cfg.validate(); err == nil {
		t.Fatal("validate() expected error for empty backends")
	}
}

func TestConfigValidate_InvalidListenAddr(t *testing.T) {
	cfg := validConfig()
	cfg.ListenAddr = "localhost"

	if err := cfg.validate(); err == nil {
		t.Fatal("validate() expected error for malformed listen address")
	}
}

func TestConfigValidate_InvalidListenPort(t *testing.T) {
	tests := []struct {
		name string
		addr string
	}{
		{name: "non-numeric port", addr: ":abc"},
		{name: "zero port", addr: ":0"},
		{name: "negative port", addr: ":-1"},
		{name: "port above maximum", addr: ":65536"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			cfg.ListenAddr = tt.addr

			if err := cfg.validate(); err == nil {
				t.Fatalf("validate() expected error for listen address %q", tt.addr)
			}
		})
	}
}

func TestConfigValidate_InvalidBackendScheme(t *testing.T) {
	cfg := validConfig()
	cfg.Backends = []string{"ftp://localhost:8081"}

	if err := cfg.validate(); err == nil {
		t.Fatal("validate() expected error for unsupported backend scheme")
	}
}

func TestConfigValidate_BackendMissingHost(t *testing.T) {
	cfg := validConfig()
	cfg.Backends = []string{"http://"}

	if err := cfg.validate(); err == nil {
		t.Fatal("validate() expected error for backend missing host")
	}
}

func TestConfigValidate_InvalidBackendPort(t *testing.T) {
	cfg := validConfig()
	cfg.Backends = []string{"http://localhost:abc"}

	if err := cfg.validate(); err == nil {
		t.Fatal("validate() expected error for invalid backend port")
	}
}

func TestConfigValidate_BackendWithoutExplicitPort(t *testing.T) {
	cfg := validConfig()
	cfg.Backends = []string{"http://localhost"}

	if err := cfg.validate(); err != nil {
		t.Fatalf("validate() returned unexpected error: %v", err)
	}
}

func TestConfigValidate_NonPositiveRequestTimeout(t *testing.T) {
	tests := []time.Duration{0, -1 * time.Second}

	for _, timeout := range tests {
		t.Run(timeout.String(), func(t *testing.T) {
			cfg := validConfig()
			cfg.RequestTimeout = timeout

			if err := cfg.validate(); err == nil {
				t.Fatalf("validate() expected error for timeout %v", timeout)
			}
		})
	}
}

func TestConfigValidate_NonPositiveHealthCheckInterval(t *testing.T) {
	cfg := validConfig()
	cfg.HealthCheckInterval = 0

	if err := cfg.validate(); err == nil {
		t.Fatal("validate() expected error for non-positive health check interval")
	}
}

func TestConfigValidate_NonPositiveHealthCheckTimeout(t *testing.T) {
	cfg := validConfig()
	cfg.HealthCheckTimeout = -1 * time.Second

	if err := cfg.validate(); err == nil {
		t.Fatal("validate() expected error for non-positive health check timeout")
	}
}
