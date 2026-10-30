# Tool Comparison

Static analysis and linting tools serve different layers of the delivery pipeline. Shipcheck focuses specifically on **pre-deployment application topology**—the connective tissue between your source code, container manifests, Compose stacks, and environment templates.

Here is an honest, candid comparison of how Shipcheck compares to other standard tools in the ecosystem:

---

## Feature Comparison Matrix

| Dimension | Shipcheck | Hadolint | Checkov | Trivy | Kube-Linter |
| :--- | :---: | :---: | :---: | :---: | :---: |
| **Primary Scope** | Application Topology & Readiness | Dockerfile Syntax & Best Practices | Infrastructure as Code (Terraform, K8s, Cloud) | Vulnerability & CVE Scanning (SBOM) | Kubernetes YAML & Helm Linting |
| **Cross-Manifest Analysis** | **Yes** (Dockerfile ↔ Compose ↔ Code ↔ Env) | No (Dockerfile only) | Limited (within IaC resources) | No (Image/FS scan) | No (K8s manifests only) |
| **Env-Var Drift Detection** | **Yes** (Code usage vs templates) | No | No | No | No |
| **Compose Topology Integrity** | **Yes** (`depends_on`, volumes, env_file) | No | Yes (Security rules) | Misconfig check | No |
| **Zero Runtime Execution** | **Yes** (Pure static Go) | Yes | Yes (Python parser) | Yes | Yes |
| **Scan Latency (Medium Repo)** | **< 5 ms** | < 20 ms | 1.5 – 5 s | 2 – 8 s | < 50 ms |
| **Offline / Air-Gapped** | **100% Offline** | 100% Offline | Offline capable | Requires CVE DB sync | 100% Offline |
| **Inline Comment Suppressions**| **Yes** (`# shipcheck:ignore`) | Yes (`# hadolint ignore`) | Yes (`# checkov:skip`) | Via `.trivyignore` | Via annotations |
| **Baseline Mode** | **Yes** (`--baseline`) | No | Via CLI flags | No | No |

---

## Detailed Breakdown

### Shipcheck vs. Hadolint
- **What Hadolint does better:** Hadolint is the industry gold standard for deep, AST-level Dockerfile linting (e.g., specific `apt-get` flag enforcement, shell formatting, and complex pipe failure analysis with ShellCheck).
- **What Shipcheck adds:** Hadolint only looks at Dockerfiles in isolation. It cannot tell you if a `COPY` path was excluded by `.dockerignore`, if the port exposed in the Dockerfile clashes with a Compose mapping, or if the `CMD` executes a dev server (`nodemon`) defined in `package.json`. Shipcheck validates the full build context and application topology.

### Shipcheck vs. Checkov
- **What Checkov does better:** Checkov provides massive compliance frameworks (SOC2, HIPAA, CIS Benchmarks) for cloud infrastructure declarations (Terraform, CloudFormation, AWS CDK, Kubernetes).
- **What Shipcheck adds:** Checkov is heavy and primarily designed for cloud infrastructure engineers. Shipcheck is a single sub-10MB binary that runs in milliseconds in local git hooks and developer terminals, specifically targeting application developers writing Docker, Compose, and application code.

### Shipcheck vs. Trivy
- **What Trivy does better:** Trivy is an exceptional vulnerability scanner (CVE scanner) that inspects installed OS packages, language dependencies, and container layer caches against live national vulnerability databases.
- **What Shipcheck adds:** Trivy tells you if `openssl` has a CVE. It does not tell you that your application will crash-loop because `DATABASE_URL` was never added to the Compose file or that PID 1 inside the container ignores `SIGTERM`. Shipcheck verifies structural deployability, not package CVE registries.

### Shipcheck vs. Kube-Linter
- **What Kube-Linter does better:** Kube-Linter excels at validating production Kubernetes manifests against cluster policies (PodSecurityStandards, replica counts, probe configuration).
- **What Shipcheck adds:** Kube-Linter only activates once you are already targeting Kubernetes. Shipcheck catches deployment defects earlier in the developer workflow—during local Compose development and container authoring.
