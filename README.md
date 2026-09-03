<!-- AGENTV1 FILE START: foundation entry point -->
# Observe Agent: portable foundation

> **Queue scope v2:** protected `observe.backend_id` and `observe.organization_id` are now required for metrics startup/check. Existing v1 spools require a verified one-time migration; subsequent endpoint/key rotation preserves backlog. See [queue migration and endpoint changes](docs/QUEUE_SCOPE_V2.md). No live deployment was performed.

> **Release layout:** local Linux/PowerShell build scripts, checksums, guarded bootstrap and tag-driven canary workflow definitions are available. See [release/distribution guide](docs/RELEASE_DISTRIBUTION.md). Only the existing Linux AMD64 DEB is customer-installable today; nothing has been pushed or published.

> **Current delivery milestone:** A Linux AMD64 **test DEB** with strict YAML configuration and a restricted systemd service is built and tested in an isolated Linux environment. See [DEB canary installation and validation](docs/DEB_CANARY.md). Installation does not start/enable collection. Live EC2/backend acceptance is still pending; do not replace the deployed Agent. The foundation-era text below is historical and superseded by this guide and the metrics/reliability documents.

> **Previous reliability phase:** Linux metrics use a bounded, persistent acknowledgement queue. See [delivery reliability](docs/DELIVERY_RELIABILITY.md), [reliability verification](docs/RELIABILITY_VALIDATION.md), and [prepared live EC2 validation](docs/LIVE_EC2_VALIDATION.md). The foundation-only and memory-only descriptions below are historical. No live deployment has been performed.

> **Current phase:** Linux metrics-only collection is implemented. See [Linux metrics runtime](docs/LINUX_METRICS.md), [configuration compatibility](docs/CONFIG_COMPATIBILITY.md), and [current validation](docs/METRICS_VALIDATION.md). The foundation description below is retained as historical context; its statements that no collector/sender exists are superseded. This is still not an approved deployed-Agent replacement.

This is a **foundation, not a deployable replacement for agent-i**. Existing Agent and Observe repositories are read-only references and were not modified. No telemetry is collected or sent by this executable. Do not replace an installed Linux Agent with this build.

Implemented: portable identity and capability policy, serialized lifecycle transitions, rollback contracts, HTTPS/API-key request construction, bounded strict configuration parsing, authenticated remote-policy gate interfaces, Linux identity/file/state primitives, Windows/Darwin compile-safe placeholders, tests and CI definition.

Not implemented: real collectors, OTLP serialization/transport loop, IMDS detector, durable telemetry queue, memory limiter, remote-policy transport/verifier/startup recovery, Windows machine identity/collectors/SCM, service installers, packages or updater. Unsupported collectors fail explicitly; they never report fabricated zeroes.

## Check the foundation

Go 1.26.7 was used for validation; module minimum is Go 1.26.0.

```sh
go test ./...
go vet ./...
go run ./cmd/observe-agent --version
go run ./cmd/observe-agent --check --config configs/agent.json
```

`--check` validates configuration only: no OS metrics, metadata lookup, secret lookup, network, listeners, or state writes. No collection daemon mode is provided yet. The example endpoint is deliberately non-routable; replace it only when an approved real sender is implemented.

The foundation encoding is **JSON, not the deployed Agent's YAML**. `agent_id: ""` semantics and environment-based authorization are preserved, but this is not a legacy parser replacement. A tested legacy YAML importer and state migration are release prerequisites. Existing `/etc/agent-i/agent.yaml` and `/etc/agent-i/env` must remain untouched.

The secret provider expects `OBSERVE_AUTHORIZATION` to contain `ApiKey <organization-scoped-key>`. Config holds only the variable name. Never commit a real key, put it in command-line arguments, print headers, or log config/request bodies.

## Documentation

- [Architecture, interfaces and diagrams](docs/ARCHITECTURE.md)
- [Customer permissions and security review](docs/PERMISSIONS_SECURITY.md)
- [Verified backend boundaries](docs/BACKEND_CONTRACT.md)
- [Upgrade and staged implementation plan](docs/UPGRADES.md)
- [Validation evidence](docs/VALIDATION.md)
- [File manifest](docs/FILE_MANIFEST.md)

Future standard OpenTelemetry components should supply commodity collection/pipeline behavior. Observe retains identity, policy, lifecycle, authorization and backend contracts. No external Go dependency is added in this foundation.
<!-- AGENTV1 FILE END -->
