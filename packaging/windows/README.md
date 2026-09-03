<!-- AGENTV1 FILE START: Windows compile-only, not an installation claim -->
# Windows (deferred)

Planned: `observe-agent_<version>_windows_amd64.zip` and `.msi`.
The binary compiles, but supported collectors, protected credential loading,
machine identity, durable state and Windows Service integration remain gates.
Do not ship a placeholder MSI/ZIP. `installers/install.ps1` fails before any mutation.
Future packaging must use restricted ACLs, preserve identity/config/state, and sign
the executable/MSI. No per-machine key or new backend identity model is implied.
<!-- AGENTV1 FILE END -->
