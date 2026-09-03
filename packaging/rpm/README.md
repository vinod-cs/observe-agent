<!-- AGENTV1 FILE START: unsupported RPM boundary -->
# RPM (deferred)

Planned: `observe-agent-<version>.x86_64.rpm` and `observe-agent-<version>.aarch64.rpm`.
No spec/installer artifact is generated yet. Prerequisites: RPM distro/service tests,
SELinux review, architecture validation, conffile and queue upgrade/removal semantics.
Reuse Linux runtime; never silently install a DEB on an RPM host. RPM Version/Release
mapping for canaries must be defined before enabling this builder.
<!-- AGENTV1 FILE END -->
