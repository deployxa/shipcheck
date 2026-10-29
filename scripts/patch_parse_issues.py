#!/usr/bin/env python3
"""Wire malformed-manifest parse issues from discovery into SC-DIS-003."""
import re, sys

def flex(old: str) -> str:
    parts = []
    for ch in old:
        if ch == "\t":
            parts.append(r"[ \t]*")
        else:
            parts.append(re.escape(ch))
    return "".join(parts)

def patch(path, old, new, label):
    s = open(path).read()
    rx = re.compile(flex(old), re.M)
    if not rx.search(s):
        print(f"FAIL: {label}")
        sys.exit(1)
    s = rx.sub(lambda m: new, s, count=1)
    open(path, "w").write(s)
    print(f"ok: {label}")

# discovery.go: add ParseIssues field
patch("internal/discovery/discovery.go",
"""	Ambiguities []Ambiguity
	Files       []fsutil.File // all readable files (for source scans)
	Errors      []fsutil.Error
}""",
"""	Ambiguities []Ambiguity
	ParseIssues []parse.Issue // manifests that exist but do not parse
	Files       []fsutil.File // all readable files (for source scans)
	Errors      []fsutil.Error
}""",
"ParseIssues field")

# discovery.go: package.json parse failure -> ParseIssues
patch("internal/discovery/discovery.go",
"""			if app, err := parsePackageJSON(f.Path); err == nil {
				app.RelDir = dir
				app.Lockfile = findLockfile(w.Files, dir)
				r.NodeApps = append(r.NodeApps, *app)
			} else {
				r.Ambiguities = append(r.Ambiguities, Ambiguity{Message: "package.json present but not valid JSON; Node analysis skipped for this directory", Where: f.RelPath})
			}""",
"""			if app, err := parsePackageJSON(f.Path); err == nil {
				app.RelDir = dir
				app.Lockfile = findLockfile(w.Files, dir)
				r.NodeApps = append(r.NodeApps, *app)
			} else {
				r.Ambiguities = append(r.Ambiguities, Ambiguity{Message: "package.json present but not valid JSON; Node analysis skipped for this directory", Where: f.RelPath})
				r.ParseIssues = append(r.ParseIssues, parse.Issue{RelPath: f.RelPath, Reason: err.Error()})
			}""",
"package.json parse issue")

# discovery.go: pyproject parse failure -> ParseIssues
patch("internal/discovery/discovery.go",
"""				} else {
					r.Ambiguities = append(r.Ambiguities, Ambiguity{Message: "pyproject.toml present but not valid TOML; limited Python analysis", Where: f.RelPath})
				}""",
"""				} else {
					r.Ambiguities = append(r.Ambiguities, Ambiguity{Message: "pyproject.toml present but not valid TOML; limited Python analysis", Where: f.RelPath})
					r.ParseIssues = append(r.ParseIssues, parse.Issue{RelPath: f.RelPath, Reason: err.Error()})
				}""",
"pyproject parse issue")

# rules: emit SC-DIS-003 for discovery parse issues
patch("internal/rules/rules.go",
"""	// Parse issues collected by earlier stages surface as SC-DIS-003.
	for _, iss := range d.Ambiguities {
		// Ambiguities are meta-only; they never fabricate findings.
		_ = iss
	}
}""",
"""	// Manifests that exist but do not parse surface as SC-DIS-003 so the
	// problem is actionable in every output format.
	seen := map[string]bool{}
	for _, iss := range d.ParseIssues {
		if seen[iss.RelPath] {
			continue
		}
		seen[iss.RelPath] = true
		e.emit("SC-DIS-003", func(f *model.Finding) {
			f.File = iss.RelPath
			f.Evidence = truncate(iss.Reason, 160)
		})
	}
}""",
"discovery parse issues -> SC-DIS-003")

print("ALL OK")
