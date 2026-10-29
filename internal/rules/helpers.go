package rules

import (
	"os"
	"path/filepath"
	"strings"
)

// osStat exists so tests can inject failures; production uses os.Stat.
var osStat = os.Stat

// newDockerIgnore loads the .dockerignore of a build context (if any) into
// an ignore stack so COPY targets excluded from the build can be detected.
func newDockerIgnore(e *engine, root, ctxDir string) *dockerIgnore {
	di := &dockerIgnore{stack: newIgnoreMatcher()}
	if ctxDir == "" {
		ctxDir = "."
	}
	rel := filepath.ToSlash(filepath.Join(ctxDir, ".dockerignore"))
	if c, ok := e.contentOf(rel); ok {
		di.stack.add(c)
	}
	return di
}

type dockerIgnore struct{ stack *ignoreMatcher }

func (d *dockerIgnore) matched(src string) bool { return d.stack.match(src) }

// ignoreMatcher is a tiny subset of dockerignore semantics: comment lines,
// blank lines, "!" negation (last match wins), trailing "/". Globs use the
// same path.Match behaviour as .gitignore support. Documented limitation:
// "**" is treated as a single-level "*".
type ignoreMatcher struct{ pats []ipat }

type ipat struct {
	glob   string
	negate bool
}

func newIgnoreMatcher() *ignoreMatcher { return &ignoreMatcher{} }

func (m *ignoreMatcher) add(content string) {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		p := ipat{}
		if strings.HasPrefix(line, "!") {
			p.negate = true
			line = strings.TrimPrefix(line, "!")
		}
		line = strings.TrimSuffix(line, "/")
		line = strings.TrimPrefix(line, "/")
		line = strings.ReplaceAll(line, "**", "*")
		if line == "" {
			continue
		}
		p.glob = line
		m.pats = append(m.pats, p)
	}
}

func (m *ignoreMatcher) match(path string) bool {
	ignored := false
	for _, p := range m.pats {
		if ok, _ := filepath.Match(p.glob, path); ok {
			ignored = !p.negate
			continue
		}
		// A pattern without a slash also matches any basename.
		if !strings.ContainsAny(p.glob, "/") {
			if ok, _ := filepath.Match(p.glob, filepath.Base(path)); ok {
				ignored = !p.negate
			}
		}
	}
	return ignored
}
