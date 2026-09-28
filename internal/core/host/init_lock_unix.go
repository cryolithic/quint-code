//go:build darwin || linux || freebsd || netbsd || openbsd || dragonfly

package host

import (
	"fmt"
	"os"
	"syscall"
)

// Locking the existing project directory needs no preflight-side write. It
// serializes cooperating init calls; external editors do not take this lock.
func lockInitRoot(root string) (func(), error) {
	f, err := os.Open(root)
	if err != nil {
		return nil, err
	}
	err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil {
		f.Close()
		if err == syscall.EWOULDBLOCK || err == syscall.EAGAIN {
			return nil, fmt.Errorf("host_init_busy: another init holds the project lock")
		}
		return nil, err
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, nil
}
