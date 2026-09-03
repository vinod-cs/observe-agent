<!-- AGENTV1 FILE START -->
# Installer boundary

Linux: `install.sh` delegates locally to the self-contained repository-root bootstrap used by the planned GitHub raw URL. It supports Debian/Ubuntu AMD64 only, pins downloads to a tag, verifies checksums/package metadata and preserves config/state. It never starts the service or asks for keys. Windows: `install.ps1` explicitly fails without mutation until runtime/package support is implemented. See [release/distribution guide](../docs/RELEASE_DISTRIBUTION.md). No installer has been published remotely.
<!-- AGENTV1 FILE END -->
