# Threat model and security boundaries

Shipcheck analyzes repositories that may be **hostile**: cloned from
untrusted sources, contributed by strangers, or simply adversarial test
cases. This document states exactly what Shipcheck does, what it refuses to
do, and where the boundaries are.

## Assets and adversaries

- **Asset 1 — the machine running Shipcheck.** Adversary: malicious project
  content (manifests, scripts, symlinks, huge/deep trees).
- **Asset 2 — project secrets.** Adversary: leaking credential *values*
  through Shipcheck output, logs, or reports.

## Hard guarantees (static mode)

Shipcheck static analysis **never**:

1. Executes project lifecycle scripts (`preinstall`, `prepare`, `start`, …),
   or any script found in the repository. *Proof:* canary test
   `TestStaticModeNeverExecutesProjectCode` — the `security-traps` fixture
   contains lifecycle scripts that would create canary files; the test
   asserts the canaries never appear, locally or in the system temp dir.
2. Imports project modules, or loads dynamic plugins.
3. Runs package managers (`npm install`, `pip install`, …) or any repository
   binary. In fact it does not even need them installed.
4. Reads files outside the analyzed project root. Every path derived from
   file *contents* (Dockerfile COPY targets, compose binds, config paths) is
   re-verified with `fsutil.Resolve` → `fsutil.Contains` before use; paths
   escaping the root are dropped, never resolved on the host.
5. Sends anything over the network. No account, no API key, no telemetry.
6. Prints a full credential value. `internal/secrets` masks every match
   (short prefix + asterisks + length) before it can reach a finding; the
   tests assert the raw value never survives.

## Defensive limits

| Vector | Mitigation |
| --- | --- |
| Path traversal (`../` in COPY/binds/config) | `fsutil.Resolve` confines every content-derived path; escapes rejected, not followed |
| Symlink escape | walker resolves symlinks; anything leaving the root is skipped and recorded as an error |
| Oversized files | files > 5 MiB are skipped with an error entry (scan becomes incomplete) |
| Deep trees | walking is capped at 24 levels; deeper subtrees are skipped and reported |
| Huge repositories | capped at 20 000 files per scan, then stops with an error entry |
| Malformed YAML/JSON/TOML | decoders return explicit errors; malformed manifests surface as finding `SC-DIS-003`, never a panic |
| Binary/unreadable files | sniffed (NUL byte) or failed reads → recorded, excluded from analysis |
| Irregular files (sockets, devices, FIFOs) | never opened |
| `docker compose config` (opt-in) | subprocess with bounded timeout (45 s), no build/pull/run flags, runs in the compose file's directory; Docker socket is never mounted anywhere |
| Output file flag | written only to the user-specified path; content is the report |

## Known boundaries (not guarantees)

- `.gitignore` support covers the common subset (comments, negation, dir
  marker, `*` globs). Exotic patterns may cause over- or under-inclusion;
  under-inclusion is safe (file scanned anyway) — over-inclusion of build
  artifacts is prevented by the built-in exclusion list.
- The `.dockerignore` reader implements the same subset.
- Archive files (`.tgz`, `.zip`) inside the project are not unpacked.
- Shipcheck reads text; encrypted or encoded secrets (base64 blobs that are
  not shaped like known tokens) are out of scope by design — see
  [false-positives.md](false-positives.md) for what secret detection claims
  and does not claim.
- `--docker-validate` runs a real external tool; it inherits whatever the
  Docker CLI does with a `config` subcommand (parse + interpolate). It never
  receives build/run instructions from Shipcheck.

## Reporting vulnerabilities

See [SECURITY.md](../SECURITY.md). Please do not open public issues for
security problems.
