package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	dir := t.TempDir()
	res, err := Load(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Path != "" {
		t.Fatal("no config file should yield empty path")
	}
	if !res.Config.SkipDotenv() {
		t.Fatal("skip_real_dotenv must default to true")
	}
}

func TestLoadStrictUnknownKeys(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, ".shipcheck.yml")
	os.WriteFile(p, []byte("severity_threshold: high\nunknown_key: 1\n"), 0o600)
	_, err := Load(dir, "")
	if err == nil {
		t.Fatal("unknown key must be rejected")
	}
}

func TestLoadValidConfig(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "shipcheck.yml")
	os.WriteFile(p, []byte("severity_threshold: LOW\nexclude: [\"legacy/*\"]\ndisable_rules: [SC-GRD-001]\nskip_real_dotenv: false\n"), 0o600)
	res, err := Load(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Config.SeverityThreshold != "low" {
		t.Fatalf("threshold not normalized: %q", res.Config.SeverityThreshold)
	}
	if res.Config.SkipDotenv() {
		t.Fatal("explicit skip_real_dotenv: false ignored")
	}
	if !res.Config.IsDisabled("SC-GRD-001") || res.Config.IsDisabled("SC-DEP-001") {
		t.Fatal("disable_rules not honoured")
	}
}

func TestInvalidThresholdWarns(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, ".shipcheck.yml")
	os.WriteFile(p, []byte("severity_threshold: banana\n"), 0o600)
	res, err := Load(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Config.SeverityThreshold != "" || len(res.Warnings) == 0 {
		t.Fatal("bad threshold must be neutralized with a warning")
	}
}

func TestConfigOutsideRootRejected(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "x.yml")
	os.WriteFile(outside, []byte("severity_threshold: low\n"), 0o600)
	if _, err := Load(dir, outside); err == nil {
		t.Fatal("config outside root must be rejected")
	}
}
