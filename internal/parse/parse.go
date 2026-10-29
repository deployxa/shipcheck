// Package parse turns untrusted configuration files into plain data
// structures. Parsing is decoupled from rule evaluation so every parser is
// independently testable and malformed input degrades to an explicit error
// instead of a panic.
package parse

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"
)

// Issue is a parse-level problem attached to a specific file.
type Issue struct {
	RelPath string
	Reason  string
}

func (i Issue) Error() string { return fmt.Sprintf("%s: %s", i.RelPath, i.Reason) }

// JSON parses a JSON document into a generic map.
func JSON(data []byte) (map[string]any, error) {
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	return m, nil
}

// YAML parses a YAML document into a generic map. yaml.v3 performs no code
// execution and rejects duplicate keys by default in strict contexts; we use
// the standard decoder and additionally limit document size upstream.
func YAML(data []byte) (map[string]any, error) {
	var m map[string]any
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("invalid YAML: %w", err)
	}
	return m, nil
}

// YAMLAny parses arbitrary YAML (used for compose where top level is a map,
// but keeps failure explicit).
func YAMLAny(data []byte) (any, error) {
	var v any
	if err := yaml.Unmarshal(data, &v); err != nil {
		return nil, fmt.Errorf("invalid YAML: %w", err)
	}
	return v, nil
}

// TOML parses a TOML document (pyproject.toml) into a generic map.
func TOML(data []byte) (map[string]any, error) {
	var m map[string]any
	if err := toml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("invalid TOML: %w", err)
	}
	return m, nil
}

// EnvFile parses KEY=VALUE pairs the way dotenv implementations commonly do:
// comments (#), blank lines, optional "export " prefix, optional quoting.
// Values are returned verbatim (quotes stripped) — callers must never print
// values, only names.
func EnvFile(content string) map[string]string {
	out := map[string]string{}
	for _, raw := range strings.Split(content, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		eq := strings.Index(line, "=")
		if eq <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		val := strings.TrimSpace(line[eq+1:])
		val = strings.TrimSuffix(val, " # inline comment marker is not stripped")
		if len(val) >= 2 && (val[0] == '"' || val[0] == '\'') && val[len(val)-1] == val[0] {
			val = val[1 : len(val)-1]
		}
		if !validEnvKey(key) {
			continue
		}
		out[key] = val
	}
	return out
}

func validEnvKey(k string) bool {
	if k == "" {
		return false
	}
	for i, r := range k {
		ok := r == '_' || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (i > 0 && r >= '0' && r <= '9')
		if !ok {
			return false
		}
	}
	return true
}

// DockerInstruction is one logical instruction with its source line.
type DockerInstruction struct {
	Cmd       string // upper-cased instruction name (FROM, RUN, CMD…)
	Args      string // raw argument text, continuations joined
	Line      int    // 1-based line number of the instruction start
	JSONForm  bool   // CMD/ENTRYPOINT written in exec (JSON array) form
	ShellForm bool
}

// Dockerfile parses a Dockerfile into logical instructions. Line
// continuations ("\" at end of line) are joined; comments are dropped.
// Unknown/malformed lines are ignored rather than fatal — the rules layer
// decides what matters.
func Dockerfile(content string) []DockerInstruction {
	var out []DockerInstruction
	lines := strings.Split(content, "\n")
	i := 0
	for i < len(lines) {
		raw := strings.TrimRight(lines[i], "\r")
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			i++
			continue
		}
		start := i
		var parts []string
		cur := raw
		for {
			cont := strings.HasSuffix(strings.TrimRight(cur, " \t"), "\\")
			if cont {
				cur = strings.TrimRight(strings.TrimRight(cur, " \t"), "\\")
			}
			parts = append(parts, cur)
			if !cont || i+1 >= len(lines) {
				break
			}
			i++
			cur = strings.TrimRight(lines[i], "\r")
		}
		joined := strings.TrimSpace(strings.Join(parts, " "))
		sp := strings.IndexAny(joined, " \t")
		if sp < 0 {
			i++
			continue
		}
		cmd := strings.ToUpper(joined[:sp])
		args := strings.TrimSpace(joined[sp:])
		inst := DockerInstruction{Cmd: cmd, Args: args, Line: start + 1}
		if cmd == "CMD" || cmd == "ENTRYPOINT" {
			s := strings.TrimSpace(args)
			inst.JSONForm = strings.HasPrefix(s, "[")
			inst.ShellForm = !inst.JSONForm
		}
		out = append(out, inst)
		i++
	}
	return out
}

// DockerJSONArgs parses the exec-form argument array of CMD/ENTRYPOINT.
func DockerJSONArgs(args string) ([]string, error) {
	var arr []string
	if err := json.Unmarshal([]byte(args), &arr); err != nil {
		return nil, err
	}
	return arr, nil
}

// Compose models the subset of docker-compose files Shipcheck reasons about.
// Any field absent from this struct is ignored, not guessed.
type Compose struct {
	Services map[string]ComposeService
	Volumes  map[string]any
	Networks map[string]any
}

type ComposeService struct {
	Name        string
	Image       string
	Build       *ComposeBuild
	Command     any // string or []any
	Entrypoint  any // string or []any
	Ports       []string
	Expose      []string
	Env         map[string]string
	EnvFiles    []string
	DependsOn   []string
	Volumes     []string
	User        string
	Privileged  bool
	NetworkMode string
	Healthcheck map[string]any
	Restart     string
}

type ComposeBuild struct {
	Context    string
	Dockerfile string
}

// ComposeFile validates that the document is a compose file (services key
// present as a map) and extracts the modeled subset.
func ComposeFile(data []byte) (*Compose, error) {
	doc, err := YAML(data)
	if err != nil {
		return nil, err
	}
	c := &Compose{Services: map[string]ComposeService{}}
	svcsRaw, ok := doc["services"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("no services map found — not a compose file")
	}
	for name, sr := range svcsRaw {
		sm, ok := sr.(map[string]any)
		if !ok {
			continue
		}
		svc := ComposeService{Name: name}
		svc.Image, _ = sm["image"].(string)
		if b, ok := sm["build"].(map[string]any); ok {
			ctx, _ := b["context"].(string)
			df, _ := b["dockerfile"].(string)
			svc.Build = &ComposeBuild{Context: ctx, Dockerfile: df}
		} else if b, ok := sm["build"].(string); ok {
			svc.Build = &ComposeBuild{Context: b, Dockerfile: "Dockerfile"}
		}
		svc.Command = sm["command"]
		svc.Entrypoint = sm["entrypoint"]
		svc.Ports = stringList(sm["ports"])
		svc.Expose = stringList(sm["expose"])
		svc.EnvFiles = envFileList(sm["env_file"])
		svc.DependsOn = dependsOnList(sm["depends_on"])
		svc.User, _ = sm["user"].(string)
		svc.Privileged, _ = sm["privileged"].(bool)
		svc.NetworkMode, _ = sm["network_mode"].(string)
		svc.Restart, _ = sm["restart"].(string)
		if hc, ok := sm["healthcheck"].(map[string]any); ok {
			svc.Healthcheck = hc
		}
		if em, ok := sm["environment"].(map[string]any); ok {
			svc.Env = map[string]string{}
			for k, v := range em {
				svc.Env[k] = fmt.Sprintf("%v", v)
			}
		} else if lst := stringList(sm["environment"]); len(lst) > 0 {
			svc.Env = map[string]string{}
			for _, item := range lst {
				if eq := strings.Index(item, "="); eq > 0 {
					svc.Env[item[:eq]] = item[eq+1:]
				} else {
					svc.Env[item] = ""
				}
			}
		}
		if vs := stringList(sm["volumes"]); len(vs) > 0 {
			svc.Volumes = vs
		}
		c.Services[name] = svc
	}
	if vm, ok := doc["volumes"].(map[string]any); ok {
		c.Volumes = vm
	}
	if nm, ok := doc["networks"].(map[string]any); ok {
		c.Networks = nm
	}
	return c, nil
}

func stringList(v any) []string {
	switch t := v.(type) {
	case []any:
		var out []string
		for _, item := range t {
			out = append(out, fmt.Sprintf("%v", item))
		}
		return out
	case string:
		return []string{t}
	}
	return nil
}

func envFileList(v any) []string {
	switch t := v.(type) {
	case []any:
		var out []string
		for _, item := range t {
			if s, ok := item.(string); ok {
				out = append(out, s)
			} else if m, ok := item.(map[string]any); ok {
				if p, ok := m["path"].(string); ok {
					out = append(out, p)
				}
			}
		}
		return out
	case string:
		return []string{t}
	}
	return nil
}

func dependsOnList(v any) []string {
	switch t := v.(type) {
	case []any:
		var out []string
		for _, item := range t {
			if s, ok := item.(string); ok {
				out = append(out, s)
			} else if m, ok := item.(map[string]any); ok {
				for k := range m {
					out = append(out, k)
				}
			}
		}
		return out
	case map[string]any:
		var out []string
		for k := range t {
			out = append(out, k)
		}
		return out
	}
	return nil
}

// PyProject extracts the modeled subset of pyproject.toml.
type PyProject struct {
	ProjectName string
	Scripts     map[string]string
	Deps        []string
}

func PyProjectFile(data []byte) (*PyProject, error) {
	doc, err := TOML(data)
	if err != nil {
		return nil, err
	}
	pp := &PyProject{Scripts: map[string]string{}}
	if p, ok := doc["project"].(map[string]any); ok {
		pp.ProjectName, _ = p["name"].(string)
		if sm, ok := p["scripts"].(map[string]any); ok {
			for k, v := range sm {
				pp.Scripts[k] = fmt.Sprintf("%v", v)
			}
		}
		if deps, ok := p["dependencies"].([]any); ok {
			for _, d := range deps {
				if s, ok := d.(string); ok {
					pp.Deps = append(pp.Deps, s)
				}
			}
		}
	}
	return pp, nil
}

// PortInHostPart extracts the host port from a compose ports entry like
// "8080:80", "127.0.0.1:8080:80/tcp", or "8080" (no mapping). Returns 0 when
// there is no explicit host port.
func PortInHostPart(entry string) int {
	s := entry
	if i := strings.Index(s, "/"); i >= 0 {
		s = s[:i]
	}
	parts := strings.Split(s, ":")
	last := parts[len(parts)-1]
	if len(parts) >= 2 {
		host := parts[len(parts)-2]
		if p, err := strconv.Atoi(host); err == nil {
			return p
		}
	}
	if p, err := strconv.Atoi(last); err == nil && len(parts) == 1 {
		return p
	}
	return 0
}

// ContainerPortOf extracts the container-side port from a ports entry.
func ContainerPortOf(entry string) int {
	s := entry
	if i := strings.Index(s, "/"); i >= 0 {
		s = s[:i]
	}
	parts := strings.Split(s, ":")
	if p, err := strconv.Atoi(parts[len(parts)-1]); err == nil {
		return p
	}
	return 0
}
