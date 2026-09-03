#!/usr/bin/env bash
# AGENTV1 FILE START: checkout entry point; root bootstrap remains self-contained.
set -euo pipefail
exec bash "$(dirname -- "${BASH_SOURCE[0]}")/../install.sh" "$@"
# AGENTV1 FILE END
