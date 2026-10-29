// Package fsutil provides the defensive filesystem primitives every other
// package must use when reading an analyzed project. An analyzed repository
// is untrusted input: walks are depth-limited, size-limited, symlink-aware
// and confined to the project root at all times.
package fsutil

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Hard limits. They exist to keep adversarial repositories cheap to scan and
// are deliberately conservative.
const (
	MaxFileSize   = 5 << 20 // refuse to read files larger than 5 MiB
	MaxDepth      = 24      // refuse to descend deeper than 24 directories
	MaxFiles      = 20000   // refuse to walk more than 20k files
	SniffLen      = 8192    // bytes inspected for binary detection
	DefaultExclud = ".git,node_modules,vendor,bower_components,.venv,venv,__pycache__,.mypy_cache,.pytest_cache,.tox,dist,build,target,out,site-packages,.next,.nuxt,.cache,.terraform,.idea,.vscode"
)

// Limits is the resolved configuration for one walk.
type Limits struct {
	Root        string
	ExcludeDirs map[string]bool
	MaxSize     int64
	MaxDepth    int
}

// DefaultLimits builds a Limits for root using the built-in exclusion list.
func DefaultLimits(root string) *Limits {
	ex := map[string]bool{}
	for _, d := range strings.Split(DefaultExclud, ",") {
		ex[d] = true
	}
	return &Limits{Root: root, ExcludeDirs: ex, MaxSize: MaxFileSize, MaxDepth: MaxDepth}
}

// Error is a non-fatal problem recorded against the scan; enough errors make
// the whole scan "incomplete" instead of silently passing.
type Error struct {
	Path    string `json:"path,omitempty"`
	Reason  string `json:"reason"`
	Skipped bool   `json:"skipped"`
}

func (e Error) String() string {
	if e.Path == "" {
		return e.Reason
	}
	return fmt.Sprintf("%s: %s", e.Path, e.Reason)
}

// Contains reports whether path p (absolute) stays inside root. It is the
// single source of truth for path-confinement checks.
func Contains(root, p string) bool {
	rel, err := filepath.Rel(root, p)
	if err != nil {
		return false
	}
	rel = filepath.ToSlash(rel)
	return rel != ".." && !strings.HasPrefix(rel, "../")
}

// Resolve joins root and rel, then verifies containment. Use for every path
// reconstructed from file contents (Dockerfile COPY targets, compose binds…)
// so a crafted "../" cannot escape the project.
func Resolve(root, rel string) (string, bool) {
	abs := filepath.Clean(filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(rel, "/"))))
	return abs, Contains(root, abs)
}

// IsBinary sniffs the first SniffLen bytes for a NUL byte.
func IsBinary(path string) (bool, error) {
	f, err := os.Open(path) // #nosec G304 -- path is produced by our confined walker
	if err != nil {
		return false, err
	}
	defer f.Close()
	buf := make([]byte, SniffLen)
	n, err := f.Read(buf)
	if err != nil && n == 0 {
		return false, nil // empty file is text
	}
	for _, b := range buf[:n] {
		if b == 0 {
			return true, nil
		}
	}
	return false, nil
}

// File is one readable text file yielded by Walk.
type File struct {
	Path    string // absolute
	RelPath string // slash-separated, relative to root
	Size    int64
}

// WalkResult is the outcome of one safe walk.
type WalkResult struct {
	Files  []File
	Errors []Error
}

// Walk traverses root breadth-first, honouring exclusions, .gitignore files,
// symlink confinement and the size/depth/file-count limits. It never follows
// symlinks that resolve outside root and never returns unreadable files —
// problems are collected in Errors so callers can report incompleteness.
func Walk(l *Limits) WalkResult {
	var res WalkResult
	ignore := NewIgnoreStack()
	seen := 0

	var walk func(dir string, rel string, depth int) bool
	walk = func(dir, rel string, depth int) bool {
		if depth > l.MaxDepth {
			res.Errors = append(res.Errors, Error{Path: rel, Reason: fmt.Sprintf("depth limit %d exceeded, subtree skipped", l.MaxDepth), Skipped: true})
			return true
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			res.Errors = append(res.Errors, Error{Path: rel, Reason: "directory unreadable: " + err.Error(), Skipped: true})
			return true
		}
		// Deterministic order.
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

		for _, e := range entries {
			name := e.Name()
			abs := filepath.Join(dir, name)
			relPath := name
			if rel != "" {
				relPath = rel + "/" + name
			}
			if seen >= MaxFiles {
				res.Errors = append(res.Errors, Error{Reason: fmt.Sprintf("file count limit %d reached, walk stopped", MaxFiles), Skipped: true})
				return false
			}

			switch e.Type() & os.ModeType {
			case os.ModeDir:
				if l.ExcludeDirs[name] || ignore.match(relPath, true) {
					continue
				}
				seen++
				if !walk(abs, relPath, depth+1) {
					return false
				}
			case os.ModeSymlink:
				seen++
				target, err := filepath.EvalSymlinks(abs)
				if err != nil || !Contains(l.Root, target) {
					res.Errors = append(res.Errors, Error{Path: relPath, Reason: "symlink escapes project root or is broken; skipped", Skipped: true})
					continue
				}
				st, err := os.Stat(target)
				if err != nil {
					continue
				}
				if st.IsDir() {
					if l.ExcludeDirs[name] || ignore.match(relPath, true) {
						continue
					}
					if !walk(abs, relPath, depth+1) {
						return false
					}
				} else {
					acceptFile(&res, &seen, l, target, relPath, ignore)
				}
			case 0: // regular file
				seen++
				acceptFile(&res, &seen, l, abs, relPath, ignore)
			default: // sockets, devices, fifos: never touched
				res.Errors = append(res.Errors, Error{Path: relPath, Reason: "irregular file type skipped", Skipped: true})
			}
		}
		return true
	}

	walk(l.Root, "", 1)
	return res
}

func acceptFile(res *WalkResult, seen *int, l *Limits, abs, relPath string, ignore *ignoreStack) {
	if ignore.match(relPath, false) {
		return
	}
	info, err := os.Lstat(abs)
	if err != nil {
		res.Errors = append(res.Errors, Error{Path: relPath, Reason: "stat failed: " + err.Error(), Skipped: true})
		return
	}
	if info.Size() > l.MaxSize {
		res.Errors = append(res.Errors, Error{Path: relPath, Reason: fmt.Sprintf("file exceeds size limit %d bytes; skipped", l.MaxSize), Skipped: true})
		return
	}
	bin, err := IsBinary(abs)
	if err != nil {
		res.Errors = append(res.Errors, Error{Path: relPath, Reason: "unreadable: " + err.Error(), Skipped: true})
		return
	}
	if bin {
		res.Errors = append(res.Errors, Error{Path: relPath, Reason: "binary file not analyzed", Skipped: false})
		return
	}
	res.Files = append(res.Files, File{Path: abs, RelPath: relPath, Size: info.Size()})
}

// ReadFile reads a text file fully, capped at MaxFileSize. Callers receive
// content plus a flag stating whether the file was truncated.
func ReadFile(path string) (string, bool, error) {
	f, err := os.Open(path) // #nosec G304 -- confined caller
	if err != nil {
		return "", false, err
	}
	defer f.Close()
	var b strings.Builder
	r := bufio.NewReader(f)
	buf := make([]byte, 64*1024)
	total := 0
	trunc := false
	for {
		n, err := r.Read(buf)
		if n > 0 {
			total += n
			if total > MaxFileSize {
				trunc = true
				break
			}
			b.Write(buf[:n])
		}
		if err != nil {
			break
		}
	}
	return b.String(), trunc, nil
}

// ---- .gitignore (minimal, documented subset) -------------------------------

// IgnoreStack is the exported handle for committed .gitignore evaluation.
type IgnoreStack struct{ patterns []ignorePat }

type ignoreStack = IgnoreStack

type ignorePat struct {
	prefix  string // literal directory prefix when no meta chars present
	glob    string
	negate  bool
	dirOnly bool
}

// NewIgnoreStack returns an empty stack.
func NewIgnoreStack() *IgnoreStack { return &IgnoreStack{} }

// Add parses one .gitignore file located at dirRel ("" for root). Only the
// documented subset is honoured: comments, blank lines, negation "!", the
// trailing "/" directory marker and simple "*" globs. Nested glob magic
// ("**") is treated as "*"; this is a documented limitation.
func (ig *IgnoreStack) Add(dirRel, content string) {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimRight(line, " \t\r")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		p := ignorePat{}
		if strings.HasPrefix(line, "!") {
			p.negate = true
			line = line[1:]
		}
		if strings.HasSuffix(line, "/") {
			p.dirOnly = true
			line = strings.TrimSuffix(line, "/")
		}
		line = strings.TrimPrefix(line, "/")
		if dirRel != "" {
			line = dirRel + "/" + line
		}
		if strings.ContainsAny(line, "*?[]") {
			p.glob = line
		} else {
			p.prefix = line
		}
		ig.patterns = append(ig.patterns, p)
	}
}

// match reports whether relPath is ignored. Last matching pattern wins,
// mirroring git semantics. dir distinguishes directories from files.
// Match reports whether relPath is ignored by the stack.
func (ig *IgnoreStack) Match(relPath string, dir bool) bool { return ig.match(relPath, dir) }

func (ig *IgnoreStack) match(relPath string, dir bool) bool {
	if relPath == "" {
		return false
	}
	ignored := false
	for _, p := range ig.patterns {
		if p.dirOnly && !dir {
			// Also ignore files inside a dir-only pattern's directory.
			if p.prefix != "" && strings.HasPrefix(relPath+"/", p.prefix+"/") {
				ignored = !p.negate
			}
			continue
		}
		if p.prefix != "" {
			if relPath == p.prefix || strings.HasPrefix(relPath+"/", p.prefix+"/") {
				ignored = !p.negate
			}
			continue
		}
		if p.glob != "" {
			if ok, _ := filepath.Match(p.glob, relPath); ok {
				ignored = !p.negate
				continue
			}
			// match any parent directory component
			parts := strings.Split(relPath, "/")
			for i := 1; i < len(parts); i++ {
				if ok, _ := filepath.Match(p.glob, strings.Join(parts[:i], "/")); ok {
					ignored = !p.negate
					break
				}
			}
		}
	}
	return ignored
}

// LoadGitignore reads the .gitignore at dir (if any) into the stack.
func LoadGitignore(ig *ignoreStack, dir, dirRel string) {
	data, _, err := ReadFile(filepath.Join(dir, ".gitignore"))
	if err == nil {
		ig.Add(dirRel, data)
	}
}

// ensure fs import used
var _ fs.FileMode = 0
