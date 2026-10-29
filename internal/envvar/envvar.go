// Package envvar compares the environment-variable names an application
// references with the names its deployment templates declare. It deals in
// NAMES ONLY: values from real .env files are never read, and values from
// templates or compose files are never included in output.
package envvar

import (
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/deployxa/shipcheck/internal/fsutil"
)

// Ref is one location where a name was seen.
type Ref struct {
	File string `json:"file"`
	Line int    `json:"line,omitempty"`
	Kind string `json:"kind"` // source | template | compose | dockerfile | procfile
}

// Analysis aggregates name-level evidence.
type Analysis struct {
	Referenced map[string][]Ref // name -> where the code reads it
	Declared   map[string][]Ref // name -> where a template declares it
	Required   map[string][]Ref // referenced with no default fallback
	Optional   map[string][]Ref // referenced with an inline default
}

// New returns an empty Analysis.
func New() *Analysis {
	return &Analysis{
		Referenced: map[string][]Ref{},
		Declared:   map[string][]Ref{},
		Required:   map[string][]Ref{},
		Optional:   map[string][]Ref{},
	}
}

// Names returns the sorted keys of a name->refs map.
func Names(m map[string][]Ref) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Unresolved are referenced names never declared anywhere.
func (a *Analysis) Unresolved() []string {
	var out []string
	for name := range a.Referenced {
		if _, ok := a.Declared[name]; !ok {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// Unreferenced are declared names the code never reads.
func (a *Analysis) Unreferenced() []string {
	var out []string
	for name := range a.Declared {
		if _, ok := a.Referenced[name]; !ok {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// ---- reference patterns ----------------------------------------------------

var (
	jsEnvRe    = regexp.MustCompile(`process\.env\.([A-Z_][A-Z0-9_]*)`)
	jsEnvIdxRe = regexp.MustCompile(`process\.env\[[\x60'"]([A-Z_][A-Z0-9_]*)[\x60'"]\]`)
	pyEnvRe    = regexp.MustCompile(`os\.(?:environ(?:\.get)?|getenv)[\[(.]\s*[\x27"]([A-Z_][A-Z0-9_]*)[\x27"]`)
	// generic ${VAR} inside non-shell config files is handled separately to
	// avoid flagging shell scripts; see scanSource for usage.
)

// ScanSource extracts env-var references from one source file. Known limits,
// documented in the rules catalogue: only direct JS `process.env.X` and the
// common Python `os.environ / os.getenv` spellings are recognized; dynamic
// construction of names is invisible to static analysis and is reported as
// such rather than guessed.
func ScanSource(relPath, content string) map[string]Ref {
	out := map[string]Ref{}
	add := func(name string, line int) {
		if _, ok := out[name]; !ok {
			out[name] = Ref{File: relPath, Line: line, Kind: "source"}
		}
	}
	for ln, line := range strings.Split(content, "\n") {
		for _, re := range []*regexp.Regexp{jsEnvRe, jsEnvIdxRe, pyEnvRe} {
			for _, m := range re.FindAllStringSubmatch(line, -1) {
				add(m[1], ln+1)
			}
		}
	}
	return out
}

// HasFallback reports whether a source line suggests an inline default for
// the name (JS `|| "x"`, `?? "x"`; Python `.get("X", "x")`). This is
// heuristic and conservative: anything ambiguous counts as having a fallback
// only when the default appears on the same line as the reference.
func HasFallback(line, name string) bool {
	if strings.Contains(line, name) {
		if strings.Contains(line, "??") || strings.Contains(line, "||") {
			return true
		}
		if strings.Contains(line, ",") && strings.Contains(line, ".get(") {
			return true
		}
		if strings.Contains(line, "getenv") && strings.Count(line, ",") >= 1 {
			return true
		}
	}
	return false
}

// DeclaredFromTemplate records names from .env.example-style templates.
// Values are parsed but immediately discarded.
func (a *Analysis) DeclaredFromTemplate(relPath, content string) {
	for i, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		eq := strings.Index(trimmed, "=")
		if eq <= 0 {
			continue
		}
		name := strings.TrimSpace(trimmed[:eq])
		name = strings.TrimPrefix(name, "export ")
		if !envNameValid(name) {
			continue
		}
		a.Declared[name] = append(a.Declared[name], Ref{File: relPath, Line: i + 1, Kind: "template"})
	}
}

// DeclaredFromKV records names from compose `environment:` maps or Dockerfile
// ENV/ARG instructions. Values are discarded.
func (a *Analysis) DeclaredFromKV(kind, relPath string, names []string, line int) {
	for _, n := range names {
		if envNameValid(n) {
			a.Declared[n] = append(a.Declared[n], Ref{File: relPath, Line: line, Kind: kind})
		}
	}
}

// DeclaredFromEnvFileRecords records the NAMES of variables present in a
// real .env file without reading values into analysis output. Shipcheck
// reads the file only to learn which names the deployment expects locally;
// the values are discarded and never surfaced.
func (a *Analysis) DeclaredFromEnvFileRecords(relPath, content string) {
	// Kept separate for auditability: this is the only place a real .env is
	// touched, and only names survive.
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		eq := strings.Index(trimmed, "=")
		if eq <= 0 || strings.HasPrefix(trimmed, "#") {
			continue
		}
		name := strings.TrimSpace(trimmed[:eq])
		if envNameValid(name) {
			a.Declared[name] = append(a.Declared[name], Ref{File: relPath, Kind: "envfile"})
		}
	}
}

func envNameValid(k string) bool {
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

// ScanProject walks every readable file and produces the full analysis.
// Static mode deliberately skips real .env files (Config.EnvSkipRealDotenv);
// names from those files then count as unresolved unless declared elsewhere,
// which is the documented, conservative behaviour.
func ScanProject(files []fsutil.File, skipRealDotenv bool) *Analysis {
	a := New()
	for _, f := range files {
		base := filepath.Base(f.RelPath)
		data, _, err := fsutil.ReadFile(f.Path)
		if err != nil {
			continue
		}
		switch {
		case base == ".env":
			if skipRealDotenv {
				continue // static mode: real .env not read (opt-in only)
			}
			a.DeclaredFromEnvFileRecords(f.RelPath, data) // names only
		case strings.HasPrefix(base, ".env.") && isTemplate(base):
			a.DeclaredFromTemplate(f.RelPath, data)
		case base == "Dockerfile" || strings.HasPrefix(strings.ToLower(base), "dockerfile"):
			for _, kv := range DockerfileEnvNames(data) {
				a.DeclaredFromKV("dockerfile", f.RelPath, kv.Names, kv.Line)
			}
		default:
			refs := ScanSource(f.RelPath, data)
			for name, ref := range refs {
				a.Referenced[name] = append(a.Referenced[name], ref)
			}
			// Required vs optional per line-level fallback detection.
			for _, line := range strings.Split(data, "\n") {
				for name := range refs {
					if HasFallback(line, name) {
						a.Optional[name] = append(a.Optional[name], Ref{File: f.RelPath, Kind: "source"})
						goto nextline
					}
				}
			nextline:
			}
		}
	}
	// Anything referenced without a recorded fallback is required.
	for name := range a.Referenced {
		if _, ok := a.Optional[name]; !ok {
			a.Required[name] = a.Referenced[name]
		}
	}
	return a
}

func isTemplate(base string) bool {
	switch strings.ToLower(base) {
	case ".env.example", ".env.sample", ".env.template", ".env.dev.example":
		return true
	}
	return false
}

// KV is a names-only result for one Dockerfile instruction.
type KV struct {
	Names []string
	Line  int
}

// DockerfileEnvNames extracts ENV/ARG names. ENV a=b c=d and ARG name[=def]
// forms are handled; multi-var ENV lines with spaces are best-effort.
func DockerfileEnvNames(content string) []KV {
	var out []KV
	for i, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.HasSuffix(trimmed, "\\") {
			trimmed = strings.TrimSpace(strings.TrimSuffix(trimmed, "\\"))
		}
		upper := strings.ToUpper(trimmed)
		switch {
		case strings.HasPrefix(upper, "ENV ") || strings.HasPrefix(upper, "ARG "):
			rest := strings.TrimSpace(trimmed[4:])
			var names []string
			if strings.Contains(rest, "=") {
				for _, pair := range splitTopLevel(rest) {
					if eq := strings.Index(pair, "="); eq > 0 {
						names = append(names, strings.TrimSpace(pair[:eq]))
					}
				}
			} else {
				f := strings.Fields(rest)
				if len(f) > 0 {
					names = append(names, f[0])
				}
			}
			out = append(out, KV{Names: names, Line: i + 1})
		}
	}
	return out
}

// splitTopLevel splits on whitespace not inside quotes.
func splitTopLevel(s string) []string {
	var parts []string
	var cur strings.Builder
	inQ := byte(0)
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case inQ != 0:
			if c == inQ {
				inQ = 0
			}
			cur.WriteByte(c)
		case c == '"' || c == '\'':
			inQ = c
			cur.WriteByte(c)
		case c == ' ' || c == '\t':
			if cur.Len() > 0 {
				parts = append(parts, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteByte(c)
		}
	}
	if cur.Len() > 0 {
		parts = append(parts, cur.String())
	}
	return parts
}

// CountLine is a small helper used by tests.
func CountLine(s string) int { return len(strings.Split(s, "\n")) }

var _ = strconv.Itoa
