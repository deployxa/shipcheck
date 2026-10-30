package rules

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/deployxa/shipcheck/internal/model"
)

var (
	ignoreRe     = regexp.MustCompile(`(?i)(?:#|//|/\*|--|;)\s*shipcheck:ignore\b(?:\s+([^\r\n*]+))?`)
	ignoreNextRe = regexp.MustCompile(`(?i)(?:#|//|/\*|--|;)\s*shipcheck:ignore-next-line\b(?:\s+([^\r\n*]+))?`)
	ignoreFileRe = regexp.MustCompile(`(?i)(?:#|//|/\*|--|;)\s*shipcheck:ignore-file\b(?:\s+([^\r\n*]+))?`)
	untilRe      = regexp.MustCompile(`(?i)\b(?:until|expires)[=:](\d{4}-\d{2}-\d{2})\b`)
)

// nowFunc allows deterministic testing of expiration dates.
var nowFunc = func() time.Time {
	return time.Now().UTC()
}

// suppressionMap stores the suppression rules for a single file.
type suppressionMap struct {
	fileSuppressed   bool
	fileRules        map[string]bool
	fileExpiredUntil string

	lineRules   map[int]map[string]bool   // line -> ruleID -> true (empty string = all)
	lineExpired map[int]map[string]string // line -> ruleID -> expiredDate string
}

func parseDirective(raw string) (rules []string, expired bool, untilStr string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, false, ""
	}

	if m := untilRe.FindStringSubmatch(raw); len(m) > 0 {
		untilStr = m[1]
		_, err := time.Parse("2006-01-02", untilStr)
		if err == nil {
			today := nowFunc().Format("2006-01-02")
			if today > untilStr {
				expired = true
			}
		}
		raw = untilRe.ReplaceAllString(raw, " ")
	}

	rules = parseRuleList(raw)
	return rules, expired, untilStr
}

func parseSuppressions(content string) *suppressionMap {
	sm := &suppressionMap{
		fileRules:   make(map[string]bool),
		lineRules:   make(map[int]map[string]bool),
		lineExpired: make(map[int]map[string]string),
	}

	lines := strings.Split(content, "\n")
	for idx, line := range lines {
		lineNum := idx + 1

		// 1. File-level suppression: shipcheck:ignore-file
		if m := ignoreFileRe.FindStringSubmatch(line); len(m) > 0 {
			rules, expired, until := parseDirective(m[1])
			if expired {
				sm.fileExpiredUntil = until
			} else {
				if len(rules) == 0 {
					sm.fileSuppressed = true
				} else {
					for _, r := range rules {
						sm.fileRules[r] = true
					}
				}
			}
		}

		// 2. Same-line suppression: shipcheck:ignore
		if m := ignoreRe.FindStringSubmatch(line); len(m) > 0 {
			rules, expired, until := parseDirective(m[1])
			if expired {
				if sm.lineExpired[lineNum] == nil {
					sm.lineExpired[lineNum] = make(map[string]string)
				}
				if len(rules) == 0 {
					sm.lineExpired[lineNum][""] = until
				} else {
					for _, r := range rules {
						sm.lineExpired[lineNum][r] = until
					}
				}
			} else {
				if sm.lineRules[lineNum] == nil {
					sm.lineRules[lineNum] = make(map[string]bool)
				}
				if len(rules) == 0 {
					sm.lineRules[lineNum][""] = true
				} else {
					for _, r := range rules {
						sm.lineRules[lineNum][r] = true
					}
				}
			}
		}

		// 3. Next-line suppression: shipcheck:ignore-next-line
		if m := ignoreNextRe.FindStringSubmatch(line); len(m) > 0 {
			rules, expired, until := parseDirective(m[1])
			nextLine := lineNum + 1
			if expired {
				if sm.lineExpired[nextLine] == nil {
					sm.lineExpired[nextLine] = make(map[string]string)
				}
				if len(rules) == 0 {
					sm.lineExpired[nextLine][""] = until
				} else {
					for _, r := range rules {
						sm.lineExpired[nextLine][r] = until
					}
				}
			} else {
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

func (sm *suppressionMap) checkSuppression(ruleID string, line int) (suppressed bool, expiredDate string) {
	if sm.fileSuppressed || sm.fileRules[ruleID] {
		return true, ""
	}
	if lineMap, ok := sm.lineRules[line]; ok {
		if lineMap[""] || lineMap[ruleID] {
			return true, ""
		}
	}

	// Check if an expired suppression was present
	if sm.fileExpiredUntil != "" {
		return false, sm.fileExpiredUntil
	}
	if expMap, ok := sm.lineExpired[line]; ok {
		if d, found := expMap[ruleID]; found {
			return false, d
		}
		if d, found := expMap[""]; found {
			return false, d
		}
	}

	return false, ""
}

// filterSuppressions removes findings that have matching active inline suppressions,
// and annotates findings where the suppression has expired.
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

		suppressed, expiredDate := sm.checkSuppression(f.RuleID, f.Line)
		if suppressed {
			continue
		}

		if expiredDate != "" {
			if f.Evidence != "" {
				f.Evidence = fmt.Sprintf("%s (suppression expired on %s)", f.Evidence, expiredDate)
			} else {
				f.Evidence = fmt.Sprintf("suppression expired on %s", expiredDate)
			}
		}

		active = append(active, f)
	}

	e.findings = active
}
