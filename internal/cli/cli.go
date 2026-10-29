// Package cli wires the command-line surface: `shipcheck <project>`,
// `shipcheck rules`, `shipcheck version`. Exit codes are a documented
// contract:
//
//	0 — scan completed, no findings at/above threshold
//	1 — scan completed, findings at/above threshold exist
//	2 — invalid invocation (bad flag, missing/unreadable project, bad config)
//	3 — unexpected internal error (scan crashed)
//	4 — scan completed but analysis is incomplete (some checks could not run)
package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/deployxa/shipcheck/internal/config"
	"github.com/deployxa/shipcheck/internal/discovery"
	"github.com/deployxa/shipcheck/internal/envvar"
	"github.com/deployxa/shipcheck/internal/model"
	"github.com/deployxa/shipcheck/internal/output"
	"github.com/deployxa/shipcheck/internal/rules"
	"github.com/deployxa/shipcheck/internal/validate"
)

// Version/Commit are set at build time via -ldflags.
var (
	Version = "0.0.0-dev"
	Commit  = "unknown"
)

// Exit codes, exported for tests.
const (
	ExitOK         = 0
	ExitFindings   = 1
	ExitUsage      = 2
	ExitCrash      = 3
	ExitIncomplete = 4
)

// Main is the entrypoint; returns the process exit code.
func Main(argv []string, stdout, stderr io.Writer) int {
	if len(argv) == 0 {
		usage(stderr)
		return ExitUsage
	}
	switch argv[0] {
	case "version", "--version", "-v":
		fmt.Fprintf(stdout, "shipcheck version %s (commit %s)\n", Version, Commit)
		fmt.Fprintln(stdout, "open-source production-readiness checks — built and maintained by Deployxa")
		return ExitOK
	case "help", "--help", "-h":
		usage(stdout)
		return ExitOK
	case "rules":
		return cmdRules(argv[1:], stdout, stderr)
	}
	if strings.HasPrefix(argv[0], "-") {
		usage(stderr)
		return ExitUsage
	}
	return cmdScan(argv, stdout, stderr)
}

func usage(w io.Writer) {
	fmt.Fprint(w, `Shipcheck — open-source production-readiness checks for your application.
Built and maintained by Deployxa.

Usage:
  shipcheck <project-root> [flags]   Run production-readiness checks (static; no project code is executed)
  shipcheck rules [--format FMT]     List the rule catalogue (text|json|markdown)
  shipcheck version                  Print version
  shipcheck help                     Show this help

Flags:
  --format string              Output format: text (default), json, sarif
  --output string              Write machine-readable output to this file instead of stdout
  --severity-threshold string  Exit 1 when findings meet this severity: info|low|medium|high|critical (default "high")
  --docker-validate            Additionally run `+"`docker compose config`"+` on compose files (requires Docker;
                               never builds or runs your containers; reports honestly when Docker is unavailable)
  --config string              Path to config file (default: .shipcheck.yml in the project root)

Exit codes:
  0 no findings at/above threshold   1 findings at/above threshold
  2 invalid invocation               3 internal error
  4 analysis incomplete (still reports everything it found)

Docs: https://github.com/deployxa/shipcheck
`)
}

// ---- scan --------------------------------------------------------------------

// valueFlags are flags that consume the following argument.
var valueFlags = map[string]bool{"--format": true, "--output": true, "--severity-threshold": true, "--config": true}

// reorderArgs hoists flags (and their values) to the front so the Go flag
// package tolerates `shipcheck ./app --format json` as well as the reverse.
func reorderArgs(argv []string) []string {
	var flags, pos []string
	for i := 0; i < len(argv); i++ {
		a := argv[i]
		if strings.HasPrefix(a, "-") {
			name := a
			if eq := strings.Index(a, "="); eq >= 0 {
				name = a[:eq]
			}
			flags = append(flags, a)
			if valueFlags[name] && !strings.Contains(a, "=") && i+1 < len(argv) && !strings.HasPrefix(argv[i+1], "-") {
				i++
				flags = append(flags, argv[i])
			}
			continue
		}
		pos = append(pos, a)
	}
	return append(flags, pos...)
}

func cmdScan(argv []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("shipcheck", flag.ContinueOnError)
	fs.SetOutput(stderr)
	format := fs.String("format", "text", "output format: text|json|sarif")
	outPath := fs.String("output", "", "write report to file")
	threshold := fs.String("severity-threshold", "high", "exit-1 threshold severity")
	dockerValidate := fs.Bool("docker-validate", false, "run docker compose config on compose files")
	cfgPath := fs.String("config", "", "path to config file")
	// Accept flags both before and after the positional project root.
	if err := fs.Parse(reorderArgs(argv)); err != nil {
		return ExitUsage
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "error: exactly one project root is required")
		return ExitUsage
	}
	root := fs.Arg(0)
	abs, err := filepath.Abs(root)
	if err != nil {
		fmt.Fprintf(stderr, "error: cannot resolve project root: %v\n", err)
		return ExitUsage
	}
	st, err := os.Stat(abs)
	if err != nil || !st.IsDir() {
		fmt.Fprintf(stderr, "error: project root %q is not a readable directory\n", root)
		return ExitUsage
	}
	if *format != "text" && *format != "json" && *format != "sarif" {
		fmt.Fprintf(stderr, "error: unknown --format %q (want text, json or sarif)\n", *format)
		return ExitUsage
	}
	th := model.ParseSeverity(*threshold)
	if model.Severity(*threshold) != th && *threshold != "info" {
		fmt.Fprintf(stderr, "error: unknown --severity-threshold %q (want info, low, medium, high, critical)\n", *threshold)
		return ExitUsage
	}

	cfgRes, err := config.Load(abs, *cfgPath)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return ExitUsage
	}

	start := model.Now()
	report := runScan(abs, cfgRes, *dockerValidate, stderr)
	report.Meta.DurationSec = int64(time.Since(start).Round(time.Second) / time.Second)

	// Choose exit code before rendering so every format agrees.
	code := ExitOK
	if report.Meta.Incomplete {
		code = ExitIncomplete
	}
	if report.CountAtOrAbove(th) > 0 {
		code = ExitFindings // threshold breach wins over incompleteness (documented)
	}

	switch *format {
	case "json":
		if err := writeReport(*outPath, stdout, report, output.JSON); err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return ExitCrash
		}
	case "sarif":
		if err := writeReport(*outPath, stdout, report, output.SARIF); err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return ExitCrash
		}
	default:
		output.Text(stdout, report, th)
	}
	return code
}

// writeReport renders via fn to stdout or --output.
func writeReport(outPath string, stdout io.Writer, r *model.Report, fn func(io.Writer, *model.Report) error) error {
	if outPath == "" {
		return fn(stdout, r)
	}
	abs, err := filepath.Abs(outPath)
	if err != nil {
		return err
	}
	f, err := os.Create(abs) // #nosec G304 -- user-specified explicit path
	if err != nil {
		return fmt.Errorf("cannot write output file: %w", err)
	}
	defer f.Close()
	if err := fn(f, r); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "report written to %s\n", abs)
	return nil
}

// runScan performs discovery, rules and optional validation.
func runScan(root string, cfgRes *config.Result, dockerValidate bool, stderr io.Writer) *model.Report {
	report := model.NewReport(root, Version)
	disc := discovery.Discover(root)
	report.Meta.FilesScanned = len(disc.Files)
	projNames := projectLabels(disc)
	report.Meta.Projects = projNames
	for _, a := range disc.Ambiguities {
		report.Meta.Notes = append(report.Meta.Notes, "discovery: "+a.Message+" ("+a.Where+")")
	}
	for _, e := range disc.Errors {
		report.Meta.Errors = append(report.Meta.Errors, e.String())
	}
	for _, w := range cfgRes.Warnings {
		fmt.Fprintf(stderr, "config warning: %s\n", w)
	}

	out := rules.Run(rules.Input{Root: root, Disc: disc, Files: disc.Files, Cfg: cfgRes.Config})
	report.Findings = append(report.Findings, out.Findings...)
	report.Meta.EnvSummary = &model.EnvSummary{
		Referenced: envvarNames(out.Env.Referenced),
		Declared:   envvarNames(out.Env.Declared),
		Required:   envvarNames(out.Env.Required),
		Optional:   envvarNames(out.Env.Optional),
		Unresolved: out.Env.Unresolved(),
	}

	if dockerValidate && len(disc.Composes) > 0 {
		vres := validate.Run(validate.Options{Root: root, Compose: composeRels(disc), Timeout: validate.DefaultTimeout})
		report.Findings = append(report.Findings, vres.Findings...)
		report.Meta.Incomplete = report.Meta.Incomplete || vres.Incomplete
		for _, n := range vres.Notes {
			report.Meta.Errors = append(report.Meta.Errors, "docker-validate: "+n)
		}
	}

	if len(report.Meta.Errors) > 0 {
		report.Meta.Incomplete = true
	}
	return report
}

func projectLabels(d *discovery.Result) []string {
	var labels []string
	seen := map[string]bool{}
	for _, a := range d.NodeApps {
		l := a.RelDir
		if l == "" {
			l = "."
		}
		if !seen[l] {
			labels = append(labels, "node:"+l)
			seen[l] = true
		}
	}
	for _, p := range d.PythonApps {
		if !p.PyProject && len(p.Requirements) == 0 {
			continue
		}
		l := p.RelDir
		if l == "" {
			l = "."
		}
		if !seen[l] {
			labels = append(labels, "python:"+l)
			seen[l] = true
		}
	}
	sort.Strings(labels)
	return labels
}

func envvarNames(m map[string][]envvar.Ref) []string { return envvar.Names(m) }

func composeRels(d *discovery.Result) []string {
	var out []string
	for _, c := range d.Composes {
		out = append(out, c.RelPath)
	}
	return out
}

// ---- rules command -------------------------------------------------------------

func cmdRules(argv []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("rules", flag.ContinueOnError)
	fs.SetOutput(stderr)
	format := fs.String("format", "text", "output format: text|json|markdown")
	if err := fs.Parse(reorderArgs(argv)); err != nil {
		return ExitUsage
	}
	switch *format {
	case "json":
		type ruleJSON struct {
			ID, Title, Category, Severity, Confidence, Description, Remediation, Limitation string
		}
		out := make([]ruleJSON, 0, len(rules.Registry))
		for _, s := range rules.Registry {
			out = append(out, ruleJSON{s.ID, s.Title, string(s.Category), string(s.Severity), string(s.Confidence), s.Description, s.Remediation, s.Limitation})
		}
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		enc.SetEscapeHTML(false)
		return errExit(enc.Encode(out), stderr)
	case "markdown":
		fmt.Fprintln(stdout, "# Shipcheck rule catalogue")
		fmt.Fprintln(stdout)
		fmt.Fprintln(stdout, "Stable rule IDs. Never renamed; see docs/rules.md for the full contract.")
		fmt.Fprintln(stdout)
		for _, s := range rules.Registry {
			fmt.Fprintf(stdout, "## %s — %s\n\n", s.ID, s.Title)
			fmt.Fprintf(stdout, "- **Category**: %s\n- **Default severity**: %s\n- **Confidence**: %s\n\n", s.Category, s.Severity, s.Confidence)
			fmt.Fprintf(stdout, "%s\n\n", s.Description)
			fmt.Fprintf(stdout, "**Fix**: %s\n\n", s.Remediation)
			if s.Limitation != "" {
				fmt.Fprintf(stdout, "**Limitation**: %s\n\n", s.Limitation)
			}
		}
		return ExitOK
	default:
		for _, s := range rules.Registry {
			fmt.Fprintf(stdout, "%-12s %-9s %-9s %-18s %s\n", s.ID, s.Severity, s.Confidence, s.Category, s.Title)
		}
		return ExitOK
	}
}

func errExit(err error, stderr io.Writer) int {
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return ExitCrash
	}
	return ExitOK
}
