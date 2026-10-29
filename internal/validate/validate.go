// Package validate implements the explicit --docker-validate mode. It is
// strictly separated from static analysis: it shells out to the Docker CLI
// with a bounded timeout, never builds or runs the project's containers,
// never mounts the Docker socket anywhere, and reports honestly when Docker
// is unavailable instead of pretending validation succeeded.
package validate

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/deployxa/shipcheck/internal/model"
)

// DefaultTimeout bounds each docker invocation.
const DefaultTimeout = 45 * time.Second

// Result reports what validation actually did.
type Result struct {
	Available  bool // docker CLI answered
	Findings   []model.Finding
	Notes      []string // human-readable notes for the report meta
	Incomplete bool     // validation could not run to completion
}

// Options configures one validation pass.
type Options struct {
	Root    string
	Compose []string // compose file rel paths to validate
	Timeout time.Duration
}

// Run validates compose files with `docker compose config`. It performs no
// build, no pull, and no run: `config` only parses and interpolates.
func Run(opts Options) *Result {
	res := &Result{}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}

	if !dockerAvailable(timeout) {
		res.Incomplete = true
		res.Notes = append(res.Notes, "Docker CLI unavailable; --docker-validate did not run (reported honestly, not treated as success)")
		for _, cf := range opts.Compose {
			res.Findings = append(res.Findings, model.Finding{
				RuleID:      "SC-VAL-001",
				Title:       "Docker validation could not run",
				Description: SpecVAL001Description(),
				Severity:    model.SevInfo,
				Confidence:  model.ConfPossible,
				Check:       model.ChkInconclusive,
				File:        cf,
				Remediation: "Install/start Docker, or run validation where Docker is available.",
			})
		}
		return res
	}
	res.Available = true

	for _, cf := range opts.Compose {
		abs := filepath.Join(opts.Root, filepath.FromSlash(cf))
		dir := filepath.Dir(abs)
		file := filepath.Base(abs)
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		cmd := exec.CommandContext(ctx, "docker", "compose", "-f", file, "config", "--quiet") // #nosec G204 -- operands are fixed literals plus a confined file name
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		cancel()
		if ctx.Err() == context.DeadlineExceeded {
			res.Incomplete = true
			res.Notes = append(res.Notes, fmt.Sprintf("%s: docker compose config timed out after %s", cf, timeout))
			res.Findings = append(res.Findings, model.Finding{
				RuleID: "SC-VAL-001", Title: "Docker validation could not run",
				Description: SpecVAL001Description(), Severity: model.SevInfo,
				Confidence: model.ConfPossible, Check: model.ChkInconclusive, File: cf,
				Remediation: "Increase the timeout or validate manually.",
			})
			continue
		}
		if err != nil {
			res.Findings = append(res.Findings, model.Finding{
				RuleID: "SC-VAL-002", Title: "Compose file failed docker compose config validation",
				Description: SpecVAL002Description(), Severity: model.SevHigh,
				Confidence: model.ConfConfirmed, Check: model.ChkValidated, File: cf,
				Evidence:    truncate(string(out), 400),
				Remediation: "Apply the fix reported by docker compose config.",
			})
			continue
		}
		res.Findings = append(res.Findings, model.Finding{
			RuleID: "SC-VAL-003", Title: "Compose file validated successfully",
			Description: SpecVAL003Description(), Severity: model.SevInfo,
			Confidence: model.ConfConfirmed, Check: model.ChkValidated, File: cf,
			Remediation: "No action required.",
		})
	}
	return res
}

func dockerAvailable(timeout time.Duration) bool {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "docker", "version", "--format", "{{.Client.Version}}").CombinedOutput() // #nosec G204 -- fixed arguments
	_ = out
	if err != nil {
		return false
	}
	// The daemon must answer too; client-only success is not enough.
	ctx2, cancel2 := context.WithTimeout(context.Background(), timeout)
	defer cancel2()
	err = exec.CommandContext(ctx2, "docker", "info", "--format", "ok").Run() // #nosec G204 -- fixed arguments
	return err == nil
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

// Spec descriptions are duplicated here to avoid importing the rules
// package (which would make validate depend on rule-engine internals).

func SpecVAL001Description() string {
	return "--docker-validate was requested but Docker is unavailable or did not answer in time; validation did not run. This is reported honestly instead of being treated as success."
}
func SpecVAL002Description() string {
	return "`docker compose config` (run with an explicit timeout, without building or running anything) reported the file invalid."
}
func SpecVAL003Description() string {
	return "`docker compose config` accepted the file. Informational: records that validation ran and passed."
}
