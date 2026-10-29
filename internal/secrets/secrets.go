// Package secrets performs conservative credential-exposure detection on the
// files of an analyzed project. It is NOT a security audit and finds only
// well-known token shapes and obviously named assignments. Detected values
// are always masked before they leave this package — no full credential is
// ever placed in a finding, a log line, or a report.
package secrets

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Tier mirrors the documented classification:
//
//	confirmed   — matches a well-known token format
//	probable    — format-level indicator with a narrower false-positive margin
//	needsReview — a named assignment that could be a placeholder
type Tier string

const (
	Confirmed   Tier = "confirmed"
	Probable    Tier = "probable"
	NeedsReview Tier = "needsReview"
)

// Match is one detected candidate. Value is ALWAYS the masked form.
type Match struct {
	RuleID    string
	File      string
	Line      int
	Masked    string // masked, never the raw value
	Kind      string
	Tier      Tier
	Downgrade bool // file type lowered the tier (docs/templates)
}

// mask keeps the first 4 characters (or fewer) and hides the rest.
func mask(v string) string {
	v = strings.TrimSpace(v)
	if len(v) <= 4 {
		return strings.Repeat("*", len(v))
	}
	return v[:4] + strings.Repeat("*", 8) + fmt.Sprintf(" (len %d)", len(v))
}

type pattern struct {
	ruleID string
	re     *regexp.Regexp
	kind   string
	tier   Tier
	group  int // capture group holding the credential, 0 = whole match
}

var patterns = []pattern{
	{"SC-SEC-001", regexp.MustCompile(`AKIA[0-9A-Z]{16}`), "AWS access key id", Confirmed, 0},
	{"SC-SEC-002", regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{36,255}`), "GitHub token", Confirmed, 0},
	{"SC-SEC-003", regexp.MustCompile(`xox[baprs]-[A-Za-z0-9-]{10,250}`), "Slack token", Confirmed, 0},
	{"SC-SEC-004", regexp.MustCompile(`\b(sk|rk)_(live)_[A-Za-z0-9]{16,250}\b`), "Stripe live secret key", Confirmed, 0},
	{"SC-SEC-005", regexp.MustCompile(`AIza[0-9A-Za-z_\-]{35}`), "Google API key", Confirmed, 0},
	{"SC-SEC-006", regexp.MustCompile(`-----BEGIN (?:RSA |EC |DSA |OPENSSH |PGP )?PRIVATE KEY-----`), "embedded private key", Confirmed, 0},
	{"SC-SEC-007", regexp.MustCompile(`npm_[A-Za-z0-9]{36}`), "npm access token", Confirmed, 0},
	{"SC-SEC-008", regexp.MustCompile(`(?i)\baws_secret_access_key\b\s*[=:]\s*['"]([A-Za-z0-9/+]{40})['"]`), "AWS secret access key", Probable, 1},
	{"SC-SEC-009", regexp.MustCompile(`(?i)(api[_-]?key|secret|passwd|password|auth[_-]?token|access[_-]?token)["']?\s*[=:]\s*["']([^"'\s]{12,200})["']`), "hardcoded credential assignment", NeedsReview, 2},
	{"SC-SEC-010", regexp.MustCompile(`(?i)authorization["']?\]?\s*[:=]\s*["']Bearer\s+([A-Za-z0-9._\-]{20,250})["']`), "hardcoded bearer token", Probable, 1},
}

// placeholders are values that pattern SC-SEC-009 ignores because they are
// conventionally non-secret.
var placeholders = regexp.MustCompile(
	`(?i)^(\$\{.*\}|<.*>|\*.*|xx*|yy*|changeme|change_me|placeholder|example|sample|your[_-].*|my[_-].*|dummy|fake|test|testing|todo|fixme|none|null|nil|true|false|[0-9]+|\.{3,})$`)

var docExts = map[string]bool{".md": true, ".txt": true, ".rst": true, ".adoc": true, ".html": true, ".svg": true}

// ScanFile checks one file's content. The file's relative path decides only
// the downgrade policy; content decides everything else.
func ScanFile(relPath, content string) []Match {
	ext := strings.ToLower(filepath.Ext(relPath))
	base := strings.ToLower(filepath.Base(relPath))
	isDoc := docExts[ext]
	isTemplate := strings.HasPrefix(base, ".env.") && !strings.HasSuffix(base, ".env")
	_ = isTemplate // templates are scanned; placeholder filter handles them

	var out []Match
	for ln, line := range strings.Split(content, "\n") {
		for _, p := range patterns {
			for _, m := range p.re.FindAllStringSubmatch(line, -1) {
				idx := p.group
				raw := m[0]
				if idx < len(m) && idx > 0 {
					raw = m[idx]
				}
				if p.ruleID == "SC-SEC-009" {
					if placeholders.MatchString(raw) {
						continue
					}
					// Skip obvious interpolation and empty-ish values.
					if strings.Contains(raw, "${") || strings.Contains(raw, "process.env") {
						continue
					}
				}
				tier := p.tier
				if isDoc && tier == Confirmed {
					tier = Probable // keys in docs are usually examples; still report
				}
				out = append(out, Match{
					RuleID: p.ruleID, File: relPath, Line: ln + 1,
					Masked: mask(raw), Kind: p.kind, Tier: tier, Downgrade: isDoc,
				})
			}
		}
	}
	return out
}

// ScanAll runs ScanFile over every file and returns matches sorted by file,
// then line, then rule ID — deterministic output for byte-identical input.
func ScanAll(files []struct{ RelPath, Content string }) []Match {
	var out []Match
	for _, f := range files {
		out = append(out, ScanFile(f.RelPath, f.Content)...)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].File != out[j].File {
			return out[i].File < out[j].File
		}
		if out[i].Line != out[j].Line {
			return out[i].Line < out[j].Line
		}
		return out[i].RuleID < out[j].RuleID
	})
	return out
}
