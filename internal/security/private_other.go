//go:build !linux

// AGENTV1 FILE START: non-Linux secret-file protection is not implemented.
package security

import (
	"errors"
	"os"
)

func OpenPrivate(string) (*os.File, error) {
	return nil, errors.New("protected YAML/secret files require Linux")
}

// AGENTV1 FILE END
