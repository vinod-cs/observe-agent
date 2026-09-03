// AGENTV1 FILE START: redacted YAML credential carrier; reference modes take precedence.
package security

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
)

type Credential struct{ inline, env, file string }

func NewCredential(inline, env, file string) Credential { return Credential{inline, env, file} }
func (Credential) String() string                       { return "[credential redacted]" }
func (Credential) GoString() string                     { return "[credential redacted]" }
func (Credential) MarshalJSON() ([]byte, error)         { return []byte(`"[redacted]"`), nil }
func (c Credential) Authorization(ctx context.Context, _ string) ([]byte, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	value := c.inline
	if c.env != "" {
		value = os.Getenv(c.env)
	} else if c.file != "" {
		f, e := OpenPrivate(c.file)
		if e != nil {
			return nil, errors.New("credential reference unavailable or unsafe")
		}
		defer f.Close()
		b, e := io.ReadAll(io.LimitReader(f, 4097))
		if e != nil || len(b) > 4096 {
			return nil, errors.New("credential reference invalid")
		}
		value = strings.TrimSpace(string(b))
		Clear(b)
	}
	value = strings.TrimPrefix(value, "ApiKey ")
	for _, r := range value {
		if r < 33 || r > 126 {
			return nil, errors.New("ingestion authorization unavailable or invalid")
		}
	}
	if value == "" || len(value) > 4089 || strings.ContainsAny(value, " \t\r\n\x00") || strings.ContainsAny(value, "<>") {
		return nil, errors.New("ingestion authorization unavailable or invalid")
	}
	return []byte("ApiKey " + value), nil
}

// AGENTV1 FILE END
