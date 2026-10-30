# Configuration reference

Shipcheck works with zero configuration. An optional `.shipcheck.yml` (or
`.shipcheck.yaml`, `shipcheck.yml`, `shipcheck.yaml`) in the project root
adjusts behaviour. Unknown keys are **rejected** — a typo cannot silently
disable a rule.

```yaml
# Fail CI at a different severity than the default (high).
severity_threshold: high   # critical | high | medium | low | info

# Extra directory names to exclude from the walk, in addition to the
# built-in defaults (.git, node_modules, vendor, dist, build, …).
exclude:
  - legacy
  - playground

# Disable specific rules by stable ID.
disable_rules:
  - SC-DCK-002   # accept running as root in this repo

# Whether real .env files are read for env-var NAME collection.
# Default: true (skipped). Names from a real .env are still useful for
# env-var comparison, but Shipcheck refuses to touch values by default.
skip_real_dotenv: true

# Custom declarative rules for organization policies
custom_rules:
  - id: "CUSTOM-001"
    title: "No plaintext database passwords in compose"
    severity: "high"         # critical | high | medium | low | info
    confidence: "confirmed"  # confirmed | probable | possible
    files: ["*compose*.yml", "*compose*.yaml"]
    pattern: "POSTGRES_PASSWORD:\\s*['\"]?[a-zA-Z0-9]+"
    remediation: "Use variable interpolation or Docker secrets."
```

## Fields

| Field | Type | Default | Meaning |
| --- | --- | --- | --- |
| `severity_threshold` | string | *(CLI flag wins)* | Same vocabulary as `--severity-threshold`; the CLI flag overrides the file. |
| `exclude` | string list | *(built-ins)* | Additional directory names to skip during discovery. Names only, not globs. |
| `disable_rules` | string list | *(none)* | Rule IDs (exact, e.g. `SC-DCK-001`) that will not run. |
| `skip_real_dotenv` | bool | `true` | When `false`, Shipcheck reads real `.env` files — for their **names only** — to improve env-var matching. Values are never emitted anywhere. |
| `custom_rules` | list of objects | *(none)* | Declarative regex rules targeting specific file patterns. |

## Validation behaviour

- Unknown keys → **usage error (exit 2)**, naming the offending key.
- Unknown `severity_threshold` value → warning; default applied.
- `disable_rules` entries not shaped like `SC-…` → warning.
- A config file outside the project root → usage error.

The schema is intentionally small. Everything else is expressed as rules with
stable IDs rather than configuration surface.
