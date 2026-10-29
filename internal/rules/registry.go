// Package rules contains the deterministic rule engine. Each rule has a
// stable SC-xxx identifier, fixed default severity/confidence, an
// actionable description and a documented limitation. Rules are pure
// functions of file contents: nothing here executes project code.
package rules

import "github.com/deployxa/shipcheck/internal/model"

// Category groups rules in the catalogue.
type Category string

const (
	CatDiscovery Category = "discovery"
	CatStartup   Category = "startup"
	CatPorts     Category = "ports"
	CatDocker    Category = "dockerfile"
	CatContext   Category = "build-context"
	CatScripts   Category = "package-scripts"
	CatCompose   Category = "compose"
	CatEnv       Category = "environment"
	CatFS        Category = "filesystem"
	CatHealth    Category = "healthcheck"
	CatGraceful  Category = "graceful-shutdown"
	CatDeps      Category = "dependencies"
	CatSecrets   Category = "secrets"
	CatValidate  Category = "docker-validation"
)

// Spec is the static definition of one rule, used by the engine and by
// `shipcheck rules`.
type Spec struct {
	ID          string
	Title       string
	Category    Category
	Severity    model.Severity
	Confidence  model.Confidence
	Description string
	Remediation string
	Limitation  string
}

// Registry is the authoritative, ordered rule list. NEVER rename an ID:
// reports, SARIF consumers and CI gates depend on stability. Deprecations
// keep the ID and change the description.
var Registry = []Spec{
	{
		ID: "SC-DIS-001", Title: "No deployable manifests found", Category: CatDiscovery,
		Severity: model.SevInfo, Confidence: model.ConfPossible,
		Description: "No package.json, pyproject.toml, requirements*.txt, Dockerfile or compose file was found in the scanned directory. Shipcheck cannot say anything meaningful about deployment readiness without at least one manifest.",
		Remediation: "Point shipcheck at the directory that contains your application manifests, or add the missing manifests.",
		Limitation:  "Only the manifest types listed in docs/rules.md are recognized.",
	},
	{
		ID: "SC-DIS-002", Title: "Real .env file present in repository", Category: CatDiscovery,
		Severity: model.SevLow, Confidence: model.ConfProbable,
		Description: "A file named .env exists in the project. If it is committed, runtime secrets may leak into version control. Shipcheck does not read its values in static mode; presence alone is not proof of exposure.",
		Remediation: "Keep real environment values out of version control: commit .env.example templates instead and ensure .env is listed in .gitignore.",
		Limitation:  "Shipcheck does not inspect git history or object databases, so it cannot confirm whether the file is actually tracked.",
	},
	{
		ID: "SC-DIS-003", Title: "Configuration file could not be parsed", Category: CatDiscovery,
		Severity: model.SevMedium, Confidence: model.ConfConfirmed,
		Description: "One of the project's configuration files (package.json, compose file, pyproject.toml) is present but malformed. Deployment tooling that consumes this file will fail or skip it silently.",
		Remediation: "Fix the syntax error reported in the evidence; re-run shipcheck afterwards.",
		Limitation:  "Only structural validity is checked; semantic errors surface as empty or partial analysis.",
	},
	{
		ID: "SC-START-001", Title: "No explicit start command discoverable", Category: CatStartup,
		Severity: model.SevInfo, Confidence: model.ConfPossible,
		Description: "No start command was found in a Dockerfile CMD/ENTRYPOINT, compose command, Procfile or package.json start script. A deployment platform will have to guess how to launch the application.",
		Remediation: "Declare the production start command explicitly (Dockerfile CMD, compose command, Procfile web entry, or package.json \"start\").",
		Limitation:  "Exotic entrypoints (systemd units, custom supervisors, Makefile targets) are not detected.",
	},
	{
		ID: "SC-BLD-001", Title: "Development server used as production entrypoint", Category: CatStartup,
		Severity: model.SevHigh, Confidence: model.ConfConfirmed,
		Description: "The production entrypoint invokes a development server (next dev, vite, nodemon, webpack-dev-server, flask run, manage.py runserver, uvicorn --reload, npm run dev, …). Development servers are not built for production traffic, expose verbose errors, and often disable caching.",
		Remediation: "Run the production build and serve it with the framework's production server or a WSGI/ASGI/HTTP server.",
		Limitation:  "Detection is pattern-based on the entrypoint command string; custom dev wrappers that rename these tools are not detected.",
	},
	{
		ID: "SC-BLD-002", Title: "Entrypoint needs build output that is never built", Category: CatStartup,
		Severity: model.SevMedium, Confidence: model.ConfProbable,
		Description: "The container entrypoint references a build-output directory (dist/, build/, .next/…) but the Dockerfile never runs the project's build step, so the referenced files will not exist in the image.",
		Remediation: "Add the build command to the image build (e.g. RUN npm run build) or use a multi-stage Dockerfile with a builder stage.",
		Limitation:  "Only the common dist/build/.next/.nuxt output dirs are correlated with a package.json build script.",
	},
	{
		ID: "SC-PORT-001", Title: "Hardcoded host port in compose port mapping", Category: CatPorts,
		Severity: model.SevLow, Confidence: model.ConfPossible,
		Description: "A compose service publishes a fixed host port. Fixed host ports collide across services/environments and cannot be remapped without editing the file.",
		Remediation: "Map host ports through an environment variable, e.g. \"${WEB_PORT:-8080}:80\".",
		Limitation:  "A fixed port may be an explicit operational requirement; treat this as a hint, not a fault.",
	},
	{
		ID: "SC-PORT-002", Title: "EXPOSE does not match compose ports", Category: CatPorts,
		Severity: model.SevMedium, Confidence: model.ConfConfirmed,
		Description: "The Dockerfile EXPOSE declaration and the compose service port mapping disagree about the container-side port. One of the two files is stale; documentation value of EXPOSE is lost and health checks may target the wrong port.",
		Remediation: "Align the container-side port in the compose ports entry with the Dockerfile EXPOSE value.",
		Limitation:  "Only applies when both declarations exist; dynamic port configuration via env vars cannot be compared statically.",
	},
	{
		ID: "SC-CTX-001", Title: "Dockerfile COPY/ADD target missing from build context", Category: CatContext,
		Severity: model.SevHigh, Confidence: model.ConfConfirmed,
		Description: "A COPY or ADD instruction references a file that does not exist in the build context (and is not excluded by .dockerignore rules). The image build will fail.",
		Remediation: "Add the missing file to the repository or fix the path in the Dockerfile.",
		Limitation:  "Multi-stage COPY --from=<stage> is skipped; --from=<image> is not resolvable statically. Files created by earlier RUN steps are not tracked.",
	},
	{
		ID: "SC-CTX-002", Title: "Compose build context does not exist", Category: CatContext,
		Severity: model.SevHigh, Confidence: model.ConfConfirmed,
		Description: "The compose service build.context points to a directory that does not exist in the repository. `docker compose build` will fail.",
		Remediation: "Fix the build.context path or add the missing directory.",
		Limitation:  "Remote contexts (git URLs, OCI names) are skipped.",
	},
	{
		ID: "SC-CTX-003", Title: "Compose references a missing Dockerfile", Category: CatContext,
		Severity: model.SevHigh, Confidence: model.ConfConfirmed,
		Description: "The compose service build section names a Dockerfile that does not exist inside the build context. `docker compose build` will fail.",
		Remediation: "Fix the dockerfile path relative to the build context.",
		Limitation:  "dockerfile_inline is not evaluated.",
	},
	{
		ID: "SC-DCK-001", Title: "Base image is not pinned", Category: CatDocker,
		Severity: model.SevLow, Confidence: model.ConfPossible,
		Description: "FROM uses no tag or the :latest tag. Rebuilds can silently move to a different base image, changing behaviour and breaking reproducibility. Note: a floating tag is a reproducibility risk, NOT evidence of a vulnerability.",
		Remediation: "Pin a specific version tag (or digest) and upgrade deliberately.",
		Limitation:  "Pinning :latest-equivalent distro tags (e.g. alpine:3 → 3.x latest) is not tracked.",
	},
	{
		ID: "SC-DCK-002", Title: "Container runs as root", Category: CatDocker,
		Severity: model.SevMedium, Confidence: model.ConfProbable,
		Description: "The Dockerfile declares no USER, so the container runs as root. Root inside the container is an additional blast radius if the application is compromised. Many workloads run as root safely; this is hardening advice, not a defect by itself.",
		Remediation: "Create an unprivileged user and switch to it with USER before the entrypoint.",
		Limitation:  "A runtime override (compose user:, Kubernetes securityContext) cannot be seen from the Dockerfile alone.",
	},
	{
		ID: "SC-DCK-003", Title: "Shell-form entrypoint will not forward signals", Category: CatGraceful,
		Severity: model.SevMedium, Confidence: model.ConfProbable,
		Description: "CMD or ENTRYPOINT uses shell form, wrapping the app in /bin/sh as PID 1. SIGTERM then reaches the shell, which does not forward it; the container gets SIGKILL after the grace period, cutting requests mid-flight.",
		Remediation: "Use the exec/JSON form (CMD [\"node\",\"server.js\"]) or run the app with an init that forwards signals (docker run --init, tini).",
		Limitation:  "Apps that install their own signal handling inside the shell command are not distinguished.",
	},
	{
		ID: "SC-DCK-004", Title: "Remote script piped into a shell at build time", Category: CatDocker,
		Severity: model.SevMedium, Confidence: model.ConfProbable,
		Description: "A RUN instruction pipes curl/wget output into sh/bash. The build executes whatever the remote endpoint returns at that moment — an unverified supply-chain step in the image.",
		Remediation: "Download the artifact, verify a pinned checksum, then execute it.",
		Limitation:  "Only direct 'curl … | sh' shapes are detected.",
	},
	{
		ID: "SC-DCK-005", Title: "No HEALTHCHECK in Dockerfile", Category: CatHealth,
		Severity: model.SevInfo, Confidence: model.ConfPossible,
		Description: "The Dockerfile declares no HEALTHCHECK. Orchestrators and compose then rely on external checks or TCP probes; absence of HEALTHCHECK does not mean the app is unhealthy.",
		Remediation: "If the platform benefits from it, add a HEALTHCHECK that queries a cheap internal endpoint.",
		Limitation:  "Health managed by the orchestrator (k8s probes) makes this declaration unnecessary.",
	},
	{
		ID: "SC-SCR-001", Title: "Package script references a missing file", Category: CatScripts,
		Severity: model.SevHigh, Confidence: model.ConfConfirmed,
		Description: "A package.json script invokes a local file that does not exist in the repository. `npm run <script>` will fail at the moment you need it most — during deploy.",
		Remediation: "Fix the path in the script or add the missing file.",
		Limitation:  "Only the first path-like token of simple invocations (node, ts-node, python) is checked; shell chains and globs are skipped.",
	},
	{
		ID: "SC-CMP-001", Title: "depends_on references an undefined service", Category: CatCompose,
		Severity: model.SevHigh, Confidence: model.ConfConfirmed,
		Description: "A compose service declares depends_on for a service that is not defined in the same file. `docker compose up` fails validation.",
		Remediation: "Define the missing service or remove the dependency entry.",
		Limitation:  "depends_on across included/merged compose files is not resolved.",
	},
	{
		ID: "SC-CMP-002", Title: "Named volume is not declared at top level", Category: CatCompose,
		Severity: model.SevHigh, Confidence: model.ConfConfirmed,
		Description: "A service mounts a named volume that has no top-level volumes declaration. Newer compose implementations reject the file.",
		Remediation: "Declare the volume under the top-level volumes: key.",
		Limitation:  "External volumes (external: true) are treated as missing; document them with a comment if intentional.",
	},
	{
		ID: "SC-CMP-003", Title: "env_file points to a missing file", Category: CatCompose,
		Severity: model.SevHigh, Confidence: model.ConfConfirmed,
		Description: "A service references an env_file that does not exist in the repository. `docker compose up` fails or starts with an incomplete environment.",
		Remediation: "Add the file or fix the path; commit a *template* (e.g. .env.production.example) for documentation.",
		Limitation:  "Files generated by CI before deploy appear missing in a static scan; document such generation in the repository.",
	},
	{
		ID: "SC-CMP-004", Title: "Bind-mount source does not exist", Category: CatCompose,
		Severity: model.SevLow, Confidence: model.ConfPossible,
		Description: "A bind mount source path does not exist in the project. Docker creates missing host directories (as root) with empty content, which can silently mask a path typo.",
		Remediation: "Verify the path; create the directory in the repository if it is meant to exist.",
		Limitation:  "Absolute host paths outside the project are not resolvable and are skipped.",
	},
	{
		ID: "SC-CMP-005", Title: "Privileged container", Category: CatCompose,
		Severity: model.SevHigh, Confidence: model.ConfProbable,
		Description: "A service runs with privileged: true, giving the container near-host capabilities. Some workloads (VPN, FUSE) require it; most do not.",
		Remediation: "Remove privileged and grant only the specific capabilities needed.",
		Limitation:  "Cannot know whether the workload genuinely needs elevated rights.",
	},
	{
		ID: "SC-CMP-006", Title: "Host network mode", Category: CatCompose,
		Severity: model.SevMedium, Confidence: model.ConfProbable,
		Description: "A service uses network_mode: host, removing container/network isolation and port mapping semantics.",
		Remediation: "Prefer a bridge network with explicit port mappings unless host networking is a documented requirement.",
		Limitation:  "Host networking is a legitimate choice for some performance-sensitive workloads.",
	},
	{
		ID: "SC-CMP-007", Title: "Docker socket mounted into a container", Category: CatCompose,
		Severity: model.SevCritical, Confidence: model.ConfConfirmed,
		Description: "The host Docker socket (/var/run/docker.sock) is mounted into a service. Anything in that container can drive the host Docker daemon — effectively host root.",
		Remediation: "Expose a narrow API proxy instead, or run the needing job on the host/outside the app stack.",
		Limitation:  "None known for this pattern.",
	},
	{
		ID: "SC-CMP-008", Title: "Dangerous host path mounted", Category: CatCompose,
		Severity: model.SevHigh, Confidence: model.ConfProbable,
		Description: "A service mounts a sensitive host path (/, /etc, /var/run, /root, …) into the container.",
		Remediation: "Mount only the specific subdirectories required, read-only where possible.",
		Limitation:  "Heuristic path list; unusual but valid layouts may need exceptions.",
	},
	{
		ID: "SC-ENV-001", Title: "Referenced environment variable is not declared in any template", Category: CatEnv,
		Severity: model.SevMedium, Confidence: model.ConfProbable,
		Description: "The code reads an environment variable that no .env.example-style template, compose environment, or Dockerfile ENV/ARG declares. A missing variable at deploy time typically produces a runtime crash or a silent default. Absence from a template is not proof the variable is unset at runtime — that is why this is a probable finding, not a confirmed defect.",
		Remediation: "Add the variable to the deployment templates and document whether it is required.",
		Limitation:  "Dynamically constructed variable names and variables injected by the platform are invisible statically.",
	},
	{
		ID: "SC-ENV-002", Title: "Declared variable is never referenced by the code", Category: CatEnv,
		Severity: model.SevInfo, Confidence: model.ConfPossible,
		Description: "A template declares a variable that the scanned code never reads. It may be consumed by infrastructure the scan cannot see (reverse proxy, platform).",
		Remediation: "Remove stale declarations or document the consumer.",
		Limitation:  "Only direct process.env / os.environ style reads are detected.",
	},
	{
		ID: "SC-ENV-003", Title: "Variable declared with different values across templates", Category: CatEnv,
		Severity: model.SevLow, Confidence: model.ConfConfirmed,
		Description: "Two declaration sites give the same variable different non-placeholder values, which usually means one file is stale. Values are shown masked-never; only the file positions are reported.",
		Remediation: "Make the templates agree or explain the difference in a comment.",
		Limitation:  "Intentional per-environment templates with the same name in different directories are treated as conflicts.",
	},
	{
		ID: "SC-ENV-004", Title: "Compose interpolation variable is not declared", Category: CatEnv,
		Severity: model.SevMedium, Confidence: model.ConfProbable,
		Description: "A compose file interpolates ${VAR} but VAR is not declared in any template, making the compose file resolve differently per environment.",
		Remediation: "Declare the variable in .env.example / deployment templates or give it an inline default (${VAR:-x}).",
		Limitation:  "Variables exported by CI are common and invisible statically; use inline defaults for those cases.",
	},
	{
		ID: "SC-FS-001", Title: "Local state persistence without a volume", Category: CatFS,
		Severity: model.SevMedium, Confidence: model.ConfPossible,
		Description: "The project appears to persist data to the container filesystem (SQLite database, uploads directory) while no volume is declared. Container filesystems are ephemeral: data is lost on redeploy. This is a probable architecture concern — the write may be cache-only.",
		Remediation: "Attach a volume to the data path, or use a managed database/object store for durable state.",
		Limitation:  "Cannot distinguish durable data from disposable cache; only sqlite/uploads patterns are recognized.",
	},
	{
		ID: "SC-HLT-001", Title: "No health check evidence for long-running service", Category: CatHealth,
		Severity: model.SevInfo, Confidence: model.ConfPossible,
		Description: "A long-running compose service has no compose healthcheck, no Dockerfile HEALTHCHECK, and no obvious health endpoint in the source. The platform will consider the service ready as soon as the process starts. This is informational: many simple services run fine without one.",
		Remediation: "Add a healthcheck (compose healthcheck.test or Dockerfile HEALTHCHECK) pointing at a cheap endpoint.",
		Limitation:  "Health endpoints with unusual paths are not detected.",
	},
	{
		ID: "SC-GRD-001", Title: "No SIGTERM handling evidence in service source", Category: CatGraceful,
		Severity: model.SevLow, Confidence: model.ConfPossible,
		Description: "No SIGTERM handler was found for a long-running service. During deploys the process is stopped: without graceful shutdown, in-flight requests are cut. Evidence for this rule is inherently weak — absence of a handler in scanned files does not prove the runtime lacks one (frameworks often install it).",
		Remediation: "Handle SIGTERM: stop accepting new connections, drain, then exit (Node: process.on('SIGTERM'); Python: signal/atexit or gunicorn graceful-timeout).",
		Limitation:  "Framework-installed handlers are invisible; treat as a review hint, not a fault.",
	},
	{
		ID: "SC-DEP-001", Title: "Floating dependency version", Category: CatDeps,
		Severity: model.SevLow, Confidence: model.ConfPossible,
		Description: "A dependency is declared as *, latest, an unpinned git URL, or (Python) without a version constraint. Floating versions make builds non-reproducible. This is NOT evidence of a vulnerability — only of reduced determinism.",
		Remediation: "Pin versions (caret/tilde ranges for npm, == for Python) and rely on lockfiles for exactness.",
		Limitation:  "npm semver ranges like ^1.2.3 are considered acceptable and are not flagged.",
	},
	{
		ID: "SC-SEC-001", Title: "AWS access key id committed", Category: CatSecrets,
		Severity: model.SevCritical, Confidence: model.ConfConfirmed,
		Description: "A string matching the AWS access key id format (AKIA…) was found in a file.",
		Remediation: "Revoke/rotate the credential, remove it from the file, and load it from a secret manager or environment.",
		Limitation:  "Format match; could be a crafted example value. The value is masked in all output.",
	},
	{
		ID: "SC-SEC-002", Title: "GitHub token committed", Category: CatSecrets,
		Severity: model.SevCritical, Confidence: model.ConfConfirmed,
		Description: "A string matching the GitHub token format (ghp_/gho_/ghu_/ghs_/ghr_) was found in a file.",
		Remediation: "Revoke the token at github.com/settings/tokens and remove it from the file.",
		Limitation:  "Format match; masked in output.",
	},
	{
		ID: "SC-SEC-003", Title: "Slack token committed", Category: CatSecrets,
		Severity: model.SevCritical, Confidence: model.ConfConfirmed,
		Description: "A string matching the Slack token format (xox…) was found in a file.",
		Remediation: "Revoke the token in the Slack app settings and remove it from the file.",
		Limitation:  "Format match; masked in output.",
	},
	{
		ID: "SC-SEC-004", Title: "Stripe live secret key committed", Category: CatSecrets,
		Severity: model.SevCritical, Confidence: model.ConfConfirmed,
		Description: "A string matching the Stripe live secret key format (sk_live_…) was found in a file.",
		Remediation: "Rotate the key in the Stripe dashboard and remove it from the file.",
		Limitation:  "Format match; masked in output.",
	},
	{
		ID: "SC-SEC-005", Title: "Google API key committed", Category: CatSecrets,
		Severity: model.SevCritical, Confidence: model.ConfConfirmed,
		Description: "A string matching the Google API key format (AIza…) was found in a file.",
		Remediation: "Restrict or rotate the key in Google Cloud console and remove it from the file.",
		Limitation:  "Format match; masked in output.",
	},
	{
		ID: "SC-SEC-006", Title: "Private key material committed", Category: CatSecrets,
		Severity: model.SevCritical, Confidence: model.ConfConfirmed,
		Description: "A PEM private key block header was found in a file.",
		Remediation: "Remove the key from the repository; if it was ever committed, consider it compromised and rotate it.",
		Limitation:  "Header-only match; masked in output.",
	},
	{
		ID: "SC-SEC-007", Title: "npm access token committed", Category: CatSecrets,
		Severity: model.SevCritical, Confidence: model.ConfConfirmed,
		Description: "A string matching the npm token format (npm_…) was found in a file.",
		Remediation: "Revoke the token and remove it from the file.",
		Limitation:  "Format match; masked in output.",
	},
	{
		ID: "SC-SEC-008", Title: "AWS secret access key candidate", Category: CatSecrets,
		Severity: model.SevHigh, Confidence: model.ConfProbable,
		Description: "An assignment named like an AWS secret access key with a 40-char base64-ish value was found.",
		Remediation: "Verify and rotate if real; move credentials to environment/secret manager.",
		Limitation:  "Value shape matched, not a service-side check; masked in output.",
	},
	{
		ID: "SC-SEC-009", Title: "Hardcoded credential assignment", Category: CatSecrets,
		Severity: model.SevMedium, Confidence: model.ConfPossible,
		Description: "An assignment to a name like password/secret/api_key with a literal string value was found. Common placeholder values are filtered; what remains needs human review and may be a dev-only fixture.",
		Remediation: "Read from environment or a secret manager instead; the value is masked in output.",
		Limitation:  "Cannot distinguish test fixtures from real credentials.",
	},
	{
		ID: "SC-SEC-010", Title: "Hardcoded bearer token", Category: CatSecrets,
		Severity: model.SevHigh, Confidence: model.ConfProbable,
		Description: "An Authorization header with a literal Bearer token was found in a file.",
		Remediation: "Load the token from configuration/environment.",
		Limitation:  "Masked in output; JWTs and opaque tokens are not distinguished.",
	},
	{
		ID: "SC-VAL-001", Title: "Docker validation could not run", Category: CatValidate,
		Severity: model.SevInfo, Confidence: model.ConfPossible,
		Description: "--docker-validate was requested but Docker is unavailable or too slow to answer; validation did not run. The scan honestly reports this instead of implying success.",
		Remediation: "Install/start Docker or run validation on a machine that has it.",
		Limitation:  "Check nature is inconclusive; the finding is a report of blocked work, not a project defect.",
	},
	{
		ID: "SC-VAL-002", Title: "Compose file failed docker compose config validation", Category: CatValidate,
		Severity: model.SevHigh, Confidence: model.ConfConfirmed,
		Description: "`docker compose config` (executed with an explicit timeout, without building or running anything) reported the file invalid.",
		Remediation: "Apply the fix reported by docker compose config in the evidence.",
		Limitation:  "Requires Docker; never run in static mode.",
	},
	{
		ID: "SC-VAL-003", Title: "Compose file validated successfully", Category: CatValidate,
		Severity: model.SevInfo, Confidence: model.ConfConfirmed,
		Description: "`docker compose config` accepted the file. Informational: records that validation ran and passed.",
		Remediation: "No action required.",
		Limitation:  "Validation covers config parsing, not image builds or runtime behaviour.",
	},
}

// Spec returns the definition for an ID, or nil.
func SpecByID(id string) *Spec {
	for i := range Registry {
		if Registry[i].ID == id {
			return &Registry[i]
		}
	}
	return nil
}

// NewFinding builds a finding from a spec, allowing per-call overrides.
func (s Spec) NewFinding() model.Finding {
	return model.Finding{
		RuleID:      s.ID,
		Title:       s.Title,
		Description: s.Description,
		Severity:    s.Severity,
		Confidence:  s.Confidence,
		Check:       model.ChkStatic,
		Remediation: s.Remediation,
	}
}
