//go:build linux

// AGENTV1 FILE START: protected config/credential files; no world or group-write access.
package security

import (
	"errors"
	"os"
	"syscall"
)

func OpenPrivate(path string) (*os.File, error) {
	fd, e := syscall.Open(path, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	if e != nil {
		return nil, errors.New("protected file unavailable")
	}
	f := os.NewFile(uintptr(fd), "protected file")
	info, e := f.Stat()
	if e != nil {
		f.Close()
		return nil, errors.New("protected file unavailable")
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.Mode().IsRegular() || info.Mode().Perm()&0027 != 0 || (st.Uid != 0 && st.Uid != uint32(os.Geteuid())) || st.Nlink != 1 {
		f.Close()
		return nil, errors.New("protected file ownership or permissions invalid")
	}
	return f, nil
}

// AGENTV1 FILE END
