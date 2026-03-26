package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Server ServerConfig  `toml:"server"`
	Cache  CacheConfig   `toml:"cache"`
	Repos  []RepoConfig  `toml:"repos"`
	Locals []LocalConfig `toml:"local"`
}

type ServerConfig struct {
	Addr string `toml:"addr"`
}

type CacheConfig struct {
	Dir      string        `toml:"dir"`
	StaleTTL time.Duration `toml:"stale_ttl"`
}

type RepoConfig struct {
	Host          string `toml:"host"`
	Owner         string `toml:"owner"`
	Repo          string `toml:"repo"`
	Remote        string `toml:"remote"`
	WebhookSecret string `toml:"webhook_secret"`
}

type LocalConfig struct {
	Label string `toml:"label"`
	Path  string `toml:"path"`
}

// Key returns the canonical string key for a repo: "host/owner/repo".
func (r RepoConfig) Key() string {
	return r.Host + "/" + r.Owner + "/" + r.Repo
}

// CloneURL returns the git clone URL. Uses Remote if set, otherwise infers
// "https://{Host}/{Owner}/{Repo}.git".
func (r RepoConfig) CloneURL() string {
	if r.Remote != "" {
		return r.Remote
	}
	return "https://" + r.Host + "/" + r.Owner + "/" + r.Repo + ".git"
}

// Load parses a TOML config file and returns a validated Config.
func Load(path string) (*Config, error) {
	cfg := Default()
	if _, err := toml.DecodeFile(path, cfg); err != nil {
		return nil, fmt.Errorf("config: decode %s: %w", path, err)
	}
	if err := cfg.expand(); err != nil {
		return nil, err
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Default returns a Config with sensible defaults.
func Default() *Config {
	return &Config{
		Server: ServerConfig{
			Addr: ":8080",
		},
		Cache: CacheConfig{
			Dir:      "~/.cache/folio",
			StaleTTL: 5 * time.Minute,
		},
	}
}

// expand expands ~ in directory paths.
func (c *Config) expand() error {
	dir, err := expandHome(c.Cache.Dir)
	if err != nil {
		return fmt.Errorf("config: expand cache dir: %w", err)
	}
	c.Cache.Dir = dir

	for i := range c.Locals {
		p, err := expandHome(c.Locals[i].Path)
		if err != nil {
			return fmt.Errorf("config: expand locals[%d] path: %w", i, err)
		}
		c.Locals[i].Path = p
	}

	return nil
}

func (c *Config) validate() error {
	for i, r := range c.Repos {
		if r.Host == "" {
			return fmt.Errorf("config: repos[%d]: host is required", i)
		}
		if r.Owner == "" {
			return fmt.Errorf("config: repos[%d]: owner is required", i)
		}
		if r.Repo == "" {
			return fmt.Errorf("config: repos[%d]: repo is required", i)
		}
	}

	for i, l := range c.Locals {
		if l.Label == "" {
			return fmt.Errorf("config: locals[%d]: label is required", i)
		}
		if strings.ContainsAny(l.Label, "/\\ \t\n") {
			return fmt.Errorf("config: locals[%d]: label %q must not contain path separators or whitespace", i, l.Label)
		}
		if l.Path == "" {
			return fmt.Errorf("config: locals[%d]: path is required", i)
		}
	}

	return nil
}

func expandHome(path string) (string, error) {
	if !strings.HasPrefix(path, "~") {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, path[1:]), nil
}
