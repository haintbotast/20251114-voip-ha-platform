package config

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type APIKey struct {
	Name string `yaml:"name"`
	Key  string `yaml:"key"`
	Role string `yaml:"role"`
}

type RecordingConfig struct {
	BasePath string `yaml:"base_path"`
}

type Config struct {
	ListenAddr       string          `yaml:"listen_addr"`
	DBDSN            string          `yaml:"db_dsn"`
	XMLCurlUser      string          `yaml:"xmlcurl_basic_user"`
	XMLCurlPass      string          `yaml:"xmlcurl_basic_pass"`
	CDRAuthorization string          `yaml:"cdr_auth_token"`
	APIKeys          []APIKey        `yaml:"api_keys"`
	Recordings       RecordingConfig `yaml:"recordings"`
}

func Load(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	dec := yaml.NewDecoder(f)
	var cfg Config
	if err := dec.Decode(&cfg); err != nil {
		return nil, err
	}

	applyStringEnvOverride("VOIPADMIND_LISTEN_ADDR", &cfg.ListenAddr)
	applyStringEnvOverride("VOIPADMIND_DB_DSN", &cfg.DBDSN)
	applyStringEnvOverride("VOIPADMIND_XMLCURL_BASIC_USER", &cfg.XMLCurlUser)
	applyStringEnvOverride("VOIPADMIND_XMLCURL_BASIC_PASS", &cfg.XMLCurlPass)
	applyStringEnvOverride("VOIPADMIND_CDR_AUTH_TOKEN", &cfg.CDRAuthorization)
	applyStringEnvOverride("VOIPADMIND_RECORDINGS_BASE_PATH", &cfg.Recordings.BasePath)

	if cfg.ListenAddr == "" {
		cfg.ListenAddr = ":8080"
	}

	if err := cfg.Validate(); err != nil {
		slog.Warn("invalid configuration", "error", err)
		// TODO: Add unit tests for configuration validation error scenarios.
		return nil, err
	}

	return &cfg, nil
}

func (c *Config) Validate() error {
	var missing []string

	if c.DBDSN == "" {
		missing = append(missing, "db_dsn")
	}
	if c.XMLCurlUser == "" {
		missing = append(missing, "xmlcurl_basic_user")
	}
	if c.XMLCurlPass == "" {
		missing = append(missing, "xmlcurl_basic_pass")
	}
	if c.CDRAuthorization == "" {
		missing = append(missing, "cdr_auth_token")
	}
	if c.Recordings.BasePath == "" {
		missing = append(missing, "recordings.base_path")
	}

	if len(missing) > 0 {
		return fmt.Errorf("config validation failed: missing %s", strings.Join(missing, ", "))
	}

	return nil
}

func applyStringEnvOverride(key string, target *string) {
	if value, ok := os.LookupEnv(key); ok && value != "" {
		*target = value
	}
}
