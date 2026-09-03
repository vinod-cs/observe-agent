<!-- AGENTV1 FILE START -->
# Packaging boundary

Release wrappers and generated output layout are documented in [release/distribution](../docs/RELEASE_DISTRIBUTION.md). Existing `deb/` behavior is unchanged. `rpm/`, `windows/` and `macos/` are explicit deferred boundaries, not working installers.

The first Linux AMD64 **test-only DEB** is defined in `deb/`. See [canary installation and verification](../docs/DEB_CANARY.md). It installs a separate observe-agent service, never replaces agent-i, and never auto-starts. RPM, other platforms and production distribution remain deferred. See [upgrade gates](../docs/UPGRADES.md).
<!-- AGENTV1 FILE END -->
