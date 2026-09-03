// AGENTV1 FILE START: opaque secret references and redacted failures.
package security

import (
	"context"
	"errors"
	"os"
	"strings"
)

// Returned bytes contain the full 'ApiKey <key>' header for old headers_env compatibility.
// Caller must use briefly, clear its copy, and never format/log the returned bytes.
type SecretProvider interface {
	Authorization(context.Context, string) ([]byte, error)
}
type Environment struct{}

func (Environment) Authorization(ctx context.Context, name string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	value := os.Getenv(name)
	if !strings.HasPrefix(value, "ApiKey ") || len(value) <= 7 || len(value) > 4096 || strings.ContainsAny(value[7:], " \t\r\n\x00") {
		return nil, errors.New("ingestion authorization unavailable or invalid")
	}
	return []byte(value), nil
}
func Clear(value []byte) { clear(value) }

// AGENTV1 FILE END
