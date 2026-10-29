// Package config loads and validates the optional .shipcheck.yml project
// configuration. Unknown keys are rejected (strict decoding) so a typo
// silently disabling a rule is impossible.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Default names searched in the project root, in order.
var ConfigNames = []string{".shipcheck.yml", ".shipcheck.yaml", "shipcheck.yml", "shipcheck.yaml"}

// Config is the modeled configuration surface. Keep it small on purpose —
// every field must be documented in docs/configuration.md.
type Config struct {
	SeverityThreshold string   `yaml:"severity_threshold"`
	Exclude           []string `yaml:"exclude"`
	DisableRules      []string `yaml:"disable_rules"`
	// SkipRealDotenv controls whether real .env files are loaded for env-var
	// name collection. Default true: Shipcheck does not read real .env
	// unless the project explicitly opts in.
	SkipRealDotenv *bool `yaml:"skip_real_dotenv"`
}

// SkipDotenv resolves the effective skip_real_dotenv value.
func (c *Config) SkipDotenv() bool {
	if c == nil || c.SkipRealDotenv == nil {
		return true
	}
	return *c.SkipRealDotenv
}

// Result carries the loaded config plus any non-fatal findings about it.
type Result struct {
	Config   *Config
	Path     string // "" when no config file exists
	Warnings []string
}

// Load reads the config for a project root. explicitPath overrides discovery.
// A missing optional config yields defaults, not an error.
func Load(root, explicitPath string) (*Result, error) {
	res := &Result{Config: &Config{}}
	path := explicitPath
	if path == "" {
		for _, name := range ConfigNames {
			p := filepath.Join(root, name)
			if st, err := os.Stat(p); err == nil && !st.IsDir() {
				path = p
				break
			}
		}
	}
	if path == "" {
		return res, nil
	}
	abs, ok := absWithin(root, path)
	if !ok {
		return nil, fmt.Errorf("config path %q escapes the project root", path)
	}
	data, err := os.ReadFile(abs) // #nosec G304 -- containment verified above
	if err != nil {
		return nil, fmt.Errorf("config unreadable: %w", err)
	}
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	if err := dec.Decode(res.Config); err != nil {
		return nil, fmt.Errorf("invalid config %s: %w", filepath.Base(path), err)
	}
	res.Path = path
	res.Warnings = validate(res.Config)
	return res, nil
}

func absWithin(root, p string) (string, bool) {
	var abs string
	if filepath.IsAbs(p) {
		abs = filepath.Clean(p)
	} else {
		abs = filepath.Clean(filepath.Join(root, p))
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil || rel == ".." || filepath.HasPrefix(rel, "..") {
		return "", false
	}
	return abs, true
}

// validate checks semantic rules that the YAML schema cannot express.
func validate(c *Config) []string {
	var warns []string
	if c.SeverityThreshold != "" {
		switch strings.ToLower(c.SeverityThreshold) {
		case "critical", "high", "medium", "low", "info":
			c.SeverityThreshold = strings.ToLower(c.SeverityThreshold)
		default:
			warns = append(warns, fmt.Sprintf("unknown severity_threshold %q; using default", c.SeverityThreshold))
			c.SeverityThreshold = ""
		}
	}
	seen := map[string]bool{}
	for _, id := range c.DisableRules {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if !strings.HasPrefix(id, "SC-") {
			warns = append(warns, fmt.Sprintf("disable_rules entry %q does not look like a rule id (expected SC-…)", id))
		}
		seen[id] = true
	}
	return warns
}

// IsDisabled reports whether a rule id is disabled.
func (c *Config) IsDisabled(id string) bool {
	if c == nil {
		return false
	}
	for _, d := range c.DisableRules {
		if strings.TrimSpace(d) == id {
			return true
		}
	}
	return false
}
