package config

import (
    "os"

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

    if cfg.ListenAddr == "" {
        cfg.ListenAddr = ":8080"
    }

    return &cfg, nil
}
