package config

import (
	"os"
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	cfg := Default()
	if cfg.Server.Addr != ":8080" {
		t.Errorf("default addr = %q, want :8080", cfg.Server.Addr)
	}
	if cfg.Cache.StaleTTL != 5*time.Minute {
		t.Errorf("default stale_ttl = %v, want 5m", cfg.Cache.StaleTTL)
	}
}

func TestLoad(t *testing.T) {
	f, err := os.CreateTemp("", "folio-config-*.toml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())

	_, _ = f.WriteString(`
[server]
addr = ":9090"

[cache]
dir = "/tmp/folio-test"
stale_ttl = "10m"

[[repos]]
host  = "github.com"
owner = "golang"
repo  = "example"

[[repos]]
host           = "tangled.sh"
owner          = "alice"
repo           = "notes"
remote         = "https://tangled.sh/alice/notes.git"
webhook_secret = "s3cr3t"
`)
	f.Close()

	cfg, err := Load(f.Name())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Server.Addr != ":9090" {
		t.Errorf("addr = %q, want :9090", cfg.Server.Addr)
	}
	if cfg.Cache.Dir != "/tmp/folio-test" {
		t.Errorf("cache dir = %q, want /tmp/folio-test", cfg.Cache.Dir)
	}
	if cfg.Cache.StaleTTL != 10*time.Minute {
		t.Errorf("stale_ttl = %v, want 10m", cfg.Cache.StaleTTL)
	}
	if len(cfg.Repos) != 2 {
		t.Fatalf("repos count = %d, want 2", len(cfg.Repos))
	}

	r0 := cfg.Repos[0]
	if r0.Key() != "github.com/golang/example" {
		t.Errorf("repos[0].Key() = %q", r0.Key())
	}
	if r0.CloneURL() != "https://github.com/golang/example.git" {
		t.Errorf("repos[0].CloneURL() = %q", r0.CloneURL())
	}

	r1 := cfg.Repos[1]
	if r1.WebhookSecret != "s3cr3t" {
		t.Errorf("repos[1].WebhookSecret = %q", r1.WebhookSecret)
	}
	if r1.CloneURL() != "https://tangled.sh/alice/notes.git" {
		t.Errorf("repos[1].CloneURL() = %q", r1.CloneURL())
	}
}

func TestValidateMissingFields(t *testing.T) {
	f, err := os.CreateTemp("", "folio-config-*.toml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	_, _ = f.WriteString("[[repos]]\nowner = \"foo\"\nrepo = \"bar\"\n")
	f.Close()

	_, err = Load(f.Name())
	if err == nil {
		t.Error("expected error for missing host, got nil")
	}
}

func TestLoadLocal(t *testing.T) {
	f, err := os.CreateTemp("", "folio-config-*.toml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())

	_, _ = f.WriteString(`
[[local]]
label = "my-docs"
path  = "/tmp/my-docs"

[[local]]
label = "other"
path  = "/tmp/other"
`)
	f.Close()

	cfg, err := Load(f.Name())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Locals) != 2 {
		t.Fatalf("locals count = %d, want 2", len(cfg.Locals))
	}
	if cfg.Locals[0].Label != "my-docs" {
		t.Errorf("locals[0].Label = %q, want my-docs", cfg.Locals[0].Label)
	}
	if cfg.Locals[0].Path != "/tmp/my-docs" {
		t.Errorf("locals[0].Path = %q, want /tmp/my-docs", cfg.Locals[0].Path)
	}
}

func TestValidateLocalMissingLabel(t *testing.T) {
	f, err := os.CreateTemp("", "folio-config-*.toml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	_, _ = f.WriteString("[[local]]\npath = \"/tmp/foo\"\n")
	f.Close()

	_, err = Load(f.Name())
	if err == nil {
		t.Error("expected error for missing label, got nil")
	}
}

func TestValidateLocalMissingPath(t *testing.T) {
	f, err := os.CreateTemp("", "folio-config-*.toml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	_, _ = f.WriteString("[[local]]\nlabel = \"foo\"\n")
	f.Close()

	_, err = Load(f.Name())
	if err == nil {
		t.Error("expected error for missing path, got nil")
	}
}

func TestValidateLocalInvalidLabel(t *testing.T) {
	f, err := os.CreateTemp("", "folio-config-*.toml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	_, _ = f.WriteString("[[local]]\nlabel = \"foo/bar\"\npath = \"/tmp/x\"\n")
	f.Close()

	_, err = Load(f.Name())
	if err == nil {
		t.Error("expected error for label with path separator, got nil")
	}
}
