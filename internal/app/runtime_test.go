// AGENTV1 FILE START: disabled profiles have no collection, credentials or metadata activity.
package app

import (
	"bytes"
	"context"
	"github.com/agent-i/agent/internal/config"
	"strings"
	"testing"
	"time"
)

func TestDisabledRuntime(t *testing.T) {
	cfg, e := config.Parse(strings.NewReader(`{"exporter":{"type":"otlp_http","endpoint":"https://must-not-be-contacted.invalid/api/v1/otlp","headers_env":{"Authorization":"MISSING_DISABLED_SECRET"}},"policy":{"version":1,"enabled":{"metrics":false,"logs":false,"traces":false}}}`))
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	var out bytes.Buffer
	if e = Run(ctx, cfg, &out); e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(out.String(), `"scrapes":0`) || !strings.Contains(out.String(), `"auth_failures":0`) {
		t.Fatal("disabled runtime performed work")
	}
}

// AGENTV1 FILE END
