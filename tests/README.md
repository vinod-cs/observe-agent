<!-- AGENTV1 FILE START -->
# Test organization

Portable unit tests live beside implementation packages and run with `go test ./...` on Linux and Windows. Linux-only tests use build tags. `tests/contract` is reserved for future real legacy-YAML and OTLP golden fixtures/backend integration, not fabricated live evidence. Native execution and cross-compilation are reported separately in [validation](../docs/VALIDATION.md).
<!-- AGENTV1 FILE END -->
