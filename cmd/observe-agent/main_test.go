// AGENTV1 FILE START: CLI does not imply that a foundation build collects telemetry.
package main

import (
	"bytes"
	"testing"
)

func TestCLI(t *testing.T) {
	var out, err bytes.Buffer
	if run([]string{"--version"}, &out, &err) != 0 {
		t.Fatal(err.String())
	}
	out.Reset()
	err.Reset()
	if run(nil, &out, &err) == 0 {
		t.Fatal("unimplemented daemon claimed success")
	}
	if run([]string{"--check", "--config", "../../configs/agent.json"}, &out, &err) != 0 {
		t.Fatal(err.String())
	}
}

// AGENTV1 FILE END
