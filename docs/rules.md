# Shipcheck rule catalogue

Stable rule IDs. Never renamed; see docs/rules.md for the full contract.

## SC-DIS-001 — No deployable manifests found

- **Category**: discovery
- **Default severity**: info
- **Confidence**: possible

No package.json, pyproject.toml, requirements*.txt, Dockerfile or compose file was found in the scanned directory. Shipcheck cannot say anything meaningful about deployment readiness without at least one manifest.

**Fix**: Point shipcheck at the directory that contains your application manifests, or add the missing manifests.

**Limitation**: Only the manifest types listed in docs/rules.md are recognized.

## SC-DIS-002 — Real .env file present in repository

- **Category**: discovery
- **Default severity**: low
- **Confidence**: probable

A file named .env exists in the project. If it is committed, runtime secrets may leak into version control. Shipcheck does not read its values in static mode; presence alone is not proof of exposure.

**Fix**: Keep real environment values out of version control: commit .env.example templates instead and ensure .env is listed in .gitignore.

**Limitation**: Shipcheck does not inspect git history or object databases, so it cannot confirm whether the file is actually tracked.

## SC-DIS-003 — Configuration file could not be parsed

- **Category**: discovery
- **Default severity**: medium
- **Confidence**: confirmed

One of the project's configuration files (package.json, compose file, pyproject.toml) is present but malformed. Deployment tooling that consumes this file will fail or skip it silently.

**Fix**: Fix the syntax error reported in the evidence; re-run shipcheck afterwards.

**Limitation**: Only structural validity is checked; semantic errors surface as empty or partial analysis.

## SC-START-001 — No explicit start command discoverable

- **Category**: startup
- **Default severity**: info
- **Confidence**: possible

No start command was found in a Dockerfile CMD/ENTRYPOINT, compose command, Procfile or package.json start script. A deployment platform will have to guess how to launch the application.

**Fix**: Declare the production start command explicitly (Dockerfile CMD, compose command, Procfile web entry, or package.json "start").

**Limitation**: Exotic entrypoints (systemd units, custom supervisors, Makefile targets) are not detected.

## SC-BLD-001 — Development server used as production entrypoint

- **Category**: startup
- **Default severity**: high
- **Confidence**: confirmed

The production entrypoint invokes a development server (next dev, vite, nodemon, webpack-dev-server, flask run, manage.py runserver, uvicorn --reload, npm run dev, …). Development servers are not built for production traffic, expose verbose errors, and often disable caching.

**Fix**: Run the production build and serve it with the framework's production server or a WSGI/ASGI/HTTP server.

**Limitation**: Detection is pattern-based on the entrypoint command string; custom dev wrappers that rename these tools are not detected.

## SC-BLD-002 — Entrypoint needs build output that is never built

- **Category**: startup
- **Default severity**: medium
- **Confidence**: probable

The container entrypoint references a build-output directory (dist/, build/, .next/…) but the Dockerfile never runs the project's build step, so the referenced files will not exist in the image.

**Fix**: Add the build command to the image build (e.g. RUN npm run build) or use a multi-stage Dockerfile with a builder stage.

**Limitation**: Only the common dist/build/.next/.nuxt output dirs are correlated with a package.json build script.

## SC-PORT-001 — Hardcoded host port in compose port mapping

- **Category**: ports
- **Default severity**: low
- **Confidence**: possible

A compose service publishes a fixed host port. Fixed host ports collide across services/environments and cannot be remapped without editing the file.

**Fix**: Map host ports through an environment variable, e.g. "${WEB_PORT:-8080}:80".

**Limitation**: A fixed port may be an explicit operational requirement; treat this as a hint, not a fault.

## SC-PORT-002 — EXPOSE does not match compose ports

- **Category**: ports
- **Default severity**: medium
- **Confidence**: confirmed

The Dockerfile EXPOSE declaration and the compose service port mapping disagree about the container-side port. One of the two files is stale; documentation value of EXPOSE is lost and health checks may target the wrong port.

**Fix**: Align the container-side port in the compose ports entry with the Dockerfile EXPOSE value.

**Limitation**: Only applies when both declarations exist; dynamic port configuration via env vars cannot be compared statically.

## SC-CTX-001 — Dockerfile COPY/ADD target missing from build context

- **Category**: build-context
- **Default severity**: high
- **Confidence**: confirmed

A COPY or ADD instruction references a file that does not exist in the build context (and is not excluded by .dockerignore rules). The image build will fail.

**Fix**: Add the missing file to the repository or fix the path in the Dockerfile.

**Limitation**: Multi-stage COPY --from=<stage> is skipped; --from=<image> is not resolvable statically. Files created by earlier RUN steps are not tracked.

## SC-CTX-002 — Compose build context does not exist

- **Category**: build-context
- **Default severity**: high
- **Confidence**: confirmed

The compose service build.context points to a directory that does not exist in the repository. `docker compose build` will fail.

**Fix**: Fix the build.context path or add the missing directory.

**Limitation**: Remote contexts (git URLs, OCI names) are skipped.

## SC-CTX-003 — Compose references a missing Dockerfile

- **Category**: build-context
- **Default severity**: high
- **Confidence**: confirmed

The compose service build section names a Dockerfile that does not exist inside the build context. `docker compose build` will fail.

**Fix**: Fix the dockerfile path relative to the build context.

**Limitation**: dockerfile_inline is not evaluated.

## SC-DCK-001 — Base image is not pinned

- **Category**: dockerfile
- **Default severity**: low
- **Confidence**: possible

FROM uses no tag or the :latest tag. Rebuilds can silently move to a different base image, changing behaviour and breaking reproducibility. Note: a floating tag is a reproducibility risk, NOT evidence of a vulnerability.

**Fix**: Pin a specific version tag (or digest) and upgrade deliberately.

**Limitation**: Pinning :latest-equivalent distro tags (e.g. alpine:3 → 3.x latest) is not tracked.

## SC-DCK-002 — Container runs as root

- **Category**: dockerfile
- **Default severity**: medium
- **Confidence**: probable

The Dockerfile declares no USER, so the container runs as root. Root inside the container is an additional blast radius if the application is compromised. Many workloads run as root safely; this is hardening advice, not a defect by itself.

**Fix**: Create an unprivileged user and switch to it with USER before the entrypoint.

**Limitation**: A runtime override (compose user:, Kubernetes securityContext) cannot be seen from the Dockerfile alone.

## SC-DCK-003 — Shell-form entrypoint will not forward signals

- **Category**: graceful-shutdown
- **Default severity**: medium
- **Confidence**: probable

CMD or ENTRYPOINT uses shell form, wrapping the app in /bin/sh as PID 1. SIGTERM then reaches the shell, which does not forward it; the container gets SIGKILL after the grace period, cutting requests mid-flight.

**Fix**: Use the exec/JSON form (CMD ["node","server.js"]) or run the app with an init that forwards signals (docker run --init, tini).

**Limitation**: Apps that install their own signal handling inside the shell command are not distinguished.

## SC-DCK-004 — Remote script piped into a shell at build time

- **Category**: dockerfile
- **Default severity**: medium
- **Confidence**: probable

A RUN instruction pipes curl/wget output into sh/bash. The build executes whatever the remote endpoint returns at that moment — an unverified supply-chain step in the image.

**Fix**: Download the artifact, verify a pinned checksum, then execute it.

**Limitation**: Only direct 'curl … | sh' shapes are detected.

## SC-DCK-005 — No HEALTHCHECK in Dockerfile

- **Category**: healthcheck
- **Default severity**: info
- **Confidence**: possible

The Dockerfile declares no HEALTHCHECK. Orchestrators and compose then rely on external checks or TCP probes; absence of HEALTHCHECK does not mean the app is unhealthy.

**Fix**: If the platform benefits from it, add a HEALTHCHECK that queries a cheap internal endpoint.

**Limitation**: Health managed by the orchestrator (k8s probes) makes this declaration unnecessary.

## SC-SCR-001 — Package script references a missing file

- **Category**: package-scripts
- **Default severity**: high
- **Confidence**: confirmed

A package.json script invokes a local file that does not exist in the repository. `npm run <script>` will fail at the moment you need it most — during deploy.

**Fix**: Fix the path in the script or add the missing file.

**Limitation**: Only the first path-like token of simple invocations (node, ts-node, python) is checked; shell chains and globs are skipped.

## SC-CMP-001 — depends_on references an undefined service

- **Category**: compose
- **Default severity**: high
- **Confidence**: confirmed

A compose service declares depends_on for a service that is not defined in the same file. `docker compose up` fails validation.

**Fix**: Define the missing service or remove the dependency entry.

**Limitation**: depends_on across included/merged compose files is not resolved.

## SC-CMP-002 — Named volume is not declared at top level

- **Category**: compose
- **Default severity**: high
- **Confidence**: confirmed

A service mounts a named volume that has no top-level volumes declaration. Newer compose implementations reject the file.

**Fix**: Declare the volume under the top-level volumes: key.

**Limitation**: External volumes (external: true) are treated as missing; document them with a comment if intentional.

## SC-CMP-003 — env_file points to a missing file

- **Category**: compose
- **Default severity**: high
- **Confidence**: confirmed

A service references an env_file that does not exist in the repository. `docker compose up` fails or starts with an incomplete environment.

**Fix**: Add the file or fix the path; commit a *template* (e.g. .env.production.example) for documentation.

**Limitation**: Files generated by CI before deploy appear missing in a static scan; document such generation in the repository.

## SC-CMP-004 — Bind-mount source does not exist

- **Category**: compose
- **Default severity**: low
- **Confidence**: possible

A bind mount source path does not exist in the project. Docker creates missing host directories (as root) with empty content, which can silently mask a path typo.

**Fix**: Verify the path; create the directory in the repository if it is meant to exist.

**Limitation**: Absolute host paths outside the project are not resolvable and are skipped.

## SC-CMP-005 — Privileged container

- **Category**: compose
- **Default severity**: high
- **Confidence**: probable

A service runs with privileged: true, giving the container near-host capabilities. Some workloads (VPN, FUSE) require it; most do not.

**Fix**: Remove privileged and grant only the specific capabilities needed.

**Limitation**: Cannot know whether the workload genuinely needs elevated rights.

## SC-CMP-006 — Host network mode

- **Category**: compose
- **Default severity**: medium
- **Confidence**: probable

A service uses network_mode: host, removing container/network isolation and port mapping semantics.

**Fix**: Prefer a bridge network with explicit port mappings unless host networking is a documented requirement.

**Limitation**: Host networking is a legitimate choice for some performance-sensitive workloads.

## SC-CMP-007 — Docker socket mounted into a container

- **Category**: compose
- **Default severity**: critical
- **Confidence**: confirmed

The host Docker socket (/var/run/docker.sock) is mounted into a service. Anything in that container can drive the host Docker daemon — effectively host root.

**Fix**: Expose a narrow API proxy instead, or run the needing job on the host/outside the app stack.

**Limitation**: None known for this pattern.

## SC-CMP-008 — Dangerous host path mounted

- **Category**: compose
- **Default severity**: high
- **Confidence**: probable

A service mounts a sensitive host path (/, /etc, /var/run, /root, …) into the container.

**Fix**: Mount only the specific subdirectories required, read-only where possible.

**Limitation**: Heuristic path list; unusual but valid layouts may need exceptions.

## SC-ENV-001 — Referenced environment variable is not declared in any template

- **Category**: environment
- **Default severity**: medium
- **Confidence**: probable

The code reads an environment variable that no .env.example-style template, compose environment, or Dockerfile ENV/ARG declares. A missing variable at deploy time typically produces a runtime crash or a silent default. Absence from a template is not proof the variable is unset at runtime — that is why this is a probable finding, not a confirmed defect.

**Fix**: Add the variable to the deployment templates and document whether it is required.

**Limitation**: Dynamically constructed variable names and variables injected by the platform are invisible statically.

## SC-ENV-002 — Declared variable is never referenced by the code

- **Category**: environment
- **Default severity**: info
- **Confidence**: possible

A template declares a variable that the scanned code never reads. It may be consumed by infrastructure the scan cannot see (reverse proxy, platform).

**Fix**: Remove stale declarations or document the consumer.

**Limitation**: Only direct process.env / os.environ style reads are detected.

## SC-ENV-003 — Variable declared with different values across templates

- **Category**: environment
- **Default severity**: low
- **Confidence**: confirmed

Two declaration sites give the same variable different non-placeholder values, which usually means one file is stale. Values are shown masked-never; only the file positions are reported.

**Fix**: Make the templates agree or explain the difference in a comment.

**Limitation**: Intentional per-environment templates with the same name in different directories are treated as conflicts.

## SC-ENV-004 — Compose interpolation variable is not declared

- **Category**: environment
- **Default severity**: medium
- **Confidence**: probable

A compose file interpolates ${VAR} but VAR is not declared in any template, making the compose file resolve differently per environment.

**Fix**: Declare the variable in .env.example / deployment templates or give it an inline default (${VAR:-x}).

**Limitation**: Variables exported by CI are common and invisible statically; use inline defaults for those cases.

## SC-FS-001 — Local state persistence without a volume

- **Category**: filesystem
- **Default severity**: medium
- **Confidence**: possible

The project appears to persist data to the container filesystem (SQLite database, uploads directory) while no volume is declared. Container filesystems are ephemeral: data is lost on redeploy. This is a probable architecture concern — the write may be cache-only.

**Fix**: Attach a volume to the data path, or use a managed database/object store for durable state.

**Limitation**: Cannot distinguish durable data from disposable cache; only sqlite/uploads patterns are recognized.

## SC-HLT-001 — No health check evidence for long-running service

- **Category**: healthcheck
- **Default severity**: info
- **Confidence**: possible

A long-running compose service has no compose healthcheck, no Dockerfile HEALTHCHECK, and no obvious health endpoint in the source. The platform will consider the service ready as soon as the process starts. This is informational: many simple services run fine without one.

**Fix**: Add a healthcheck (compose healthcheck.test or Dockerfile HEALTHCHECK) pointing at a cheap endpoint.

**Limitation**: Health endpoints with unusual paths are not detected.

## SC-GRD-001 — No SIGTERM handling evidence in service source

- **Category**: graceful-shutdown
- **Default severity**: low
- **Confidence**: possible

No SIGTERM handler was found for a long-running service. During deploys the process is stopped: without graceful shutdown, in-flight requests are cut. Evidence for this rule is inherently weak — absence of a handler in scanned files does not prove the runtime lacks one (frameworks often install it).

**Fix**: Handle SIGTERM: stop accepting new connections, drain, then exit (Node: process.on('SIGTERM'); Python: signal/atexit or gunicorn graceful-timeout).

**Limitation**: Framework-installed handlers are invisible; treat as a review hint, not a fault.

## SC-DEP-001 — Floating dependency version

- **Category**: dependencies
- **Default severity**: low
- **Confidence**: possible

A dependency is declared as *, latest, an unpinned git URL, or (Python) without a version constraint. Floating versions make builds non-reproducible. This is NOT evidence of a vulnerability — only of reduced determinism.

**Fix**: Pin versions (caret/tilde ranges for npm, == for Python) and rely on lockfiles for exactness.

**Limitation**: npm semver ranges like ^1.2.3 are considered acceptable and are not flagged.

## SC-SEC-001 — AWS access key id committed

- **Category**: secrets
- **Default severity**: critical
- **Confidence**: confirmed

A string matching the AWS access key id format (AKIA…) was found in a file.

**Fix**: Revoke/rotate the credential, remove it from the file, and load it from a secret manager or environment.

**Limitation**: Format match; could be a crafted example value. The value is masked in all output.

## SC-SEC-002 — GitHub token committed

- **Category**: secrets
- **Default severity**: critical
- **Confidence**: confirmed

A string matching the GitHub token format (ghp_/gho_/ghu_/ghs_/ghr_) was found in a file.

**Fix**: Revoke the token at github.com/settings/tokens and remove it from the file.

**Limitation**: Format match; masked in output.

## SC-SEC-003 — Slack token committed

- **Category**: secrets
- **Default severity**: critical
- **Confidence**: confirmed

A string matching the Slack token format (xox…) was found in a file.

**Fix**: Revoke the token in the Slack app settings and remove it from the file.

**Limitation**: Format match; masked in output.

## SC-SEC-004 — Stripe live secret key committed

- **Category**: secrets
- **Default severity**: critical
- **Confidence**: confirmed

A string matching the Stripe live secret key format (sk_live_…) was found in a file.

**Fix**: Rotate the key in the Stripe dashboard and remove it from the file.

**Limitation**: Format match; masked in output.

## SC-SEC-005 — Google API key committed

- **Category**: secrets
- **Default severity**: critical
- **Confidence**: confirmed

A string matching the Google API key format (AIza…) was found in a file.

**Fix**: Restrict or rotate the key in Google Cloud console and remove it from the file.

**Limitation**: Format match; masked in output.

## SC-SEC-006 — Private key material committed

- **Category**: secrets
- **Default severity**: critical
- **Confidence**: confirmed

A PEM private key block header was found in a file.

**Fix**: Remove the key from the repository; if it was ever committed, consider it compromised and rotate it.

**Limitation**: Header-only match; masked in output.

## SC-SEC-007 — npm access token committed

- **Category**: secrets
- **Default severity**: critical
- **Confidence**: confirmed

A string matching the npm token format (npm_…) was found in a file.

**Fix**: Revoke the token and remove it from the file.

**Limitation**: Format match; masked in output.

## SC-SEC-008 — AWS secret access key candidate

- **Category**: secrets
- **Default severity**: high
- **Confidence**: probable

An assignment named like an AWS secret access key with a 40-char base64-ish value was found.

**Fix**: Verify and rotate if real; move credentials to environment/secret manager.

**Limitation**: Value shape matched, not a service-side check; masked in output.

## SC-SEC-009 — Hardcoded credential assignment

- **Category**: secrets
- **Default severity**: medium
- **Confidence**: possible

An assignment to a name like password/secret/api_key with a literal string value was found. Common placeholder values are filtered; what remains needs human review and may be a dev-only fixture.

**Fix**: Read from environment or a secret manager instead; the value is masked in output.

**Limitation**: Cannot distinguish test fixtures from real credentials.

## SC-SEC-010 — Hardcoded bearer token

- **Category**: secrets
- **Default severity**: high
- **Confidence**: probable

An Authorization header with a literal Bearer token was found in a file.

**Fix**: Load the token from configuration/environment.

**Limitation**: Masked in output; JWTs and opaque tokens are not distinguished.

## SC-VAL-001 — Docker validation could not run

- **Category**: docker-validation
- **Default severity**: info
- **Confidence**: possible

--docker-validate was requested but Docker is unavailable or too slow to answer; validation did not run. The scan honestly reports this instead of implying success.

**Fix**: Install/start Docker or run validation on a machine that has it.

**Limitation**: Check nature is inconclusive; the finding is a report of blocked work, not a project defect.

## SC-VAL-002 — Compose file failed docker compose config validation

- **Category**: docker-validation
- **Default severity**: high
- **Confidence**: confirmed

`docker compose config` (executed with an explicit timeout, without building or running anything) reported the file invalid.

**Fix**: Apply the fix reported by docker compose config in the evidence.

**Limitation**: Requires Docker; never run in static mode.

## SC-VAL-003 — Compose file validated successfully

- **Category**: docker-validation
- **Default severity**: info
- **Confidence**: confirmed

`docker compose config` accepted the file. Informational: records that validation ran and passed.

**Fix**: No action required.

**Limitation**: Validation covers config parsing, not image builds or runtime behaviour.

