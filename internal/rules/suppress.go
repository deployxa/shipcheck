package rules

import (
	"regexp"
	"strings"

	"github.com/deployxa/shipcheck/internal/model"
)

var (
	ignoreRe     = regexp.MustCompile(`(?i)(?:#|//|/\*|--|;)\s*shipcheck:ignore(?:\s+([A-Za-z0-9_,\s-]+))?`)
	ignoreNextRe = regexp.MustCompile(`(?i)(?:#|//|/\*|--|;)\s*shipcheck:ignore-next-line(?:\s+([A-Za-z0-9_,\s-]+))?`)
	ignoreFileRe = regexp.MustCompile(`(?i)(?:#|//|/\*|--|;)\s*shipcheck:ignore-file(?:\s+([A-Za-z0-9_,\s-]+))?`)
)

// suppressionMap stores the suppression rules for a single file.
type suppressionMap struct {
	fileSuppressed bool
	fileRules      map[string]bool
	lineRules      map[int]map[string]bool // line (1-based) -> ruleID -> true (empty string = all)
}

func parseSuppressions(content string) *suppressionMap {
	sm := &suppressionMap{
		fileRules: make(map[string]bool),
		lineRules: make(map[int]map[string]bool),
	}

	lines := strings.Split(content, "\n")
	for idx, line := range lines {
		lineNum := idx + 1

		// 1. File-level suppression
		if m := ignoreFileRe.FindStringSubmatch(line); len(m) > 0 {
			rules := parseRuleList(m[1])
			if len(rules) == 0 {
				sm.fileSuppressed = true
			} else {
				for _, r := range rules {
					sm.fileRules[r] = true
				}
			}
		}

		// 2. Same-line suppression: shipcheck:ignore
		if m := ignoreRe.FindStringSubmatch(line); len(m) > 0 {
			rules := parseRuleList(m[1])
			if sm.lineRules[lineNum] == nil {
				sm.lineRules[lineNum] = make(map[string]bool)
			}
			if len(rules) == 0 {
				sm.lineRules[lineNum][""] = true // blanket ignore for line
			} else {
				for _, r := range rules {
					sm.lineRules[lineNum][r] = true
				}
			}
		}

		// 3. Next-line suppression: shipcheck:ignore-next-line
		if m := ignoreNextRe.FindStringSubmatch(line); len(m) > 0 {
			rules := parseRuleList(m[1])
			nextLine := lineNum + 1
			if sm.lineRules[nextLine] == nil {
				sm.lineRules[nextLine] = make(map[string]bool)
			}
			if len(rules) == 0 {
				sm.lineRules[nextLine][""] = true
			} else {
				for _, r := range rules {
					sm.lineRules[nextLine][r] = true
				}
			}
		}
	}

	return sm
}

func parseRuleList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t'
	})
	var clean []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			clean = append(clean, strings.ToUpper(p))
		}
	}
	return clean
}

func (sm *suppressionMap) isSuppressed(ruleID string, line int) bool {
	if sm.fileSuppressed {
		return true
	}
	if sm.fileRules[ruleID] {
		return true
	}
	if lineMap, ok := sm.lineRules[line]; ok {
		if lineMap[""] || lineMap[ruleID] {
			return true
		}
	}
	return false
}

// filterSuppressions removes findings that have matching inline suppressions.
func (e *engine) filterSuppressions() {
	if len(e.findings) == 0 {
		return
	}

	suppressions := make(map[string]*suppressionMap)
	var active []model.Finding

	for _, f := range e.findings {
		if f.File == "" {
			active = append(active, f)
			continue
		}

		sm, ok := suppressions[f.File]
		if !ok {
			content, exists := e.contentOf(f.File)
			if !exists {
				active = append(active, f)
				continue
			}
			sm = parseSuppressions(content)
			suppressions[f.File] = sm
		}

		if !sm.isSuppressed(f.RuleID, f.Line) {
			active = append(active, f)
		}
	}

	e.findings = active
}
