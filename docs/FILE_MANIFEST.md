<!-- AGENTV1 FILE START: complete foundation manifest -->
# Files created

> This is the historical 46-file foundation manifest. The current additions/changes are listed in [metrics validation](METRICS_VALIDATION.md).

All 46 source/config/documentation files were created in `D:/ob-cs-repo/Updated-Agent-v1`. Neither reference repository was modified. No Git repository, commit, package, release or deployment was created.

```text
Updated-Agent-v1/
├── .github/
│   └── workflows/
│       └── ci.yml
├── .gitignore
├── README.md
├── cmd/
│   └── observe-agent/
│       ├── main.go
│       └── main_test.go
├── configs/
│   └── agent.json
├── deploy/
│   └── README.md
├── docs/
│   ├── ARCHITECTURE.md
│   ├── BACKEND_CONTRACT.md
│   ├── FILE_MANIFEST.md
│   ├── PERMISSIONS_SECURITY.md
│   ├── UPGRADES.md
│   └── VALIDATION.md
├── go.mod
├── installers/
│   └── README.md
├── internal/
│   ├── app/
│   │   ├── app.go
│   │   └── app_test.go
│   ├── cloud/
│   │   └── cloud.go
│   ├── collectors/
│   │   └── collectors.go
│   ├── config/
│   │   ├── config.go
│   │   └── config_test.go
│   ├── exporter/
│   │   ├── exporter.go
│   │   └── exporter_test.go
│   ├── fleet/
│   │   ├── fleet.go
│   │   └── fleet_test.go
│   ├── identity/
│   │   ├── identity.go
│   │   └── identity_test.go
│   ├── pipeline/
│   │   └── pipeline.go
│   ├── platform/
│   │   ├── native_darwin.go
│   │   ├── native_linux.go
│   │   ├── native_linux_test.go
│   │   ├── native_other.go
│   │   ├── native_windows.go
│   │   ├── platform.go
│   │   └── platform_test.go
│   ├── policy/
│   │   ├── policy.go
│   │   └── policy_test.go
│   ├── processors/
│   │   └── processors.go
│   ├── queue/
│   │   └── queue.go
│   ├── receivers/
│   │   └── receivers.go
│   ├── security/
│   │   └── security.go
│   ├── selftelemetry/
│   │   └── events.go
│   └── version/
│       └── version.go
├── packaging/
│   └── README.md
└── tests/
    ├── README.md
    └── contract/
        └── README.md
```

`dist/` is ignored generated validation output, not a distributable release. It contains isolated Go caches/temp/module directories and `bin/observe-agent_linux_amd64`, `observe-agent_linux_arm64`, `observe-agent_windows_amd64.exe`, `observe-agent_darwin_arm64`, and `platform_linux_amd64.test` (compile-only). These are foundation previews, not installed Agents.

## Boundaries

- `cmd`: safe validation/version CLI; no daemon startup.
- `app`, `policy`, `fleet`: portable staged lifecycle, local capability ceiling, authenticated-envelope gate and LKG adapter.
- `identity`, `cloud`: stable host identity and trusted detector interface.
- `config`, `security`, `exporter`: strict config, environment reference and bounded HTTPS/JSON request construction.
- `platform`: native Linux primitives and Windows/Darwin fail-closed placeholders; other OS unsupported.
- `pipeline`, `queue`, `receivers`, `processors`: OTel integration contracts only.
- `collectors`: capability registrations, no implemented collectors.
- `selftelemetry`: redacted event boundary, no active diagnostic server.
- `packaging`, `installers`, `deploy`: documented reserved boundaries, not packaging/rollout implementation.
- `tests`: future integration fixture location; implemented unit tests are colocated beside Go packages.

See [architecture](ARCHITECTURE.md), [permissions](PERMISSIONS_SECURITY.md) and [validation](VALIDATION.md).
<!-- AGENTV1 FILE END -->
