package host

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

func verifyUnchangedInitFile(root, name string, p planned) error {
	if err := safePath(root, p.path); err != nil {
		return fmt.Errorf("host_conflict: %w", err)
	}
	current, err := os.ReadFile(p.path)
	if err != nil || !bytes.Equal(current, p.before) {
		return fmt.Errorf("host_conflict: destination changed: %s", name)
	}
	return nil
}

// Each file is published completely or left at its previous bytes. The final
// read catches an intervening edit before the filesystem effect. A foreign
// editor that races after that read does not honor the project-local lock.
func publishInitFile(root, name string, p planned, beforePublish func(string) error) error {
	if err := safePath(root, p.path); err != nil {
		return fmt.Errorf("host_conflict: %w", err)
	}
	dir := filepath.Dir(p.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	if err := safePath(root, p.path); err != nil {
		return fmt.Errorf("host_conflict: %w", err)
	}
	f, err := os.CreateTemp(dir, ".haft10-init-*")
	if err != nil {
		return err
	}
	temp := f.Name()
	defer os.Remove(temp)
	// CreateTemp applies the child process's umask to its private 0600 mode.
	// Only replacements inherit the existing destination's permissions.
	if p.exists {
		info, statErr := os.Lstat(p.path)
		if statErr != nil || !info.Mode().IsRegular() {
			f.Close()
			return fmt.Errorf("host_conflict: destination changed: %s", name)
		}
		if err := f.Chmod(info.Mode().Perm()); err != nil {
			f.Close()
			return err
		}
	}
	if _, err := f.Write(p.after); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if beforePublish != nil {
		if err := beforePublish(name); err != nil {
			return err
		}
	}
	if err := safePath(root, p.path); err != nil {
		return fmt.Errorf("host_conflict: %w", err)
	}
	current, err := os.ReadFile(p.path)
	if p.exists {
		if err != nil || !bytes.Equal(current, p.before) {
			return fmt.Errorf("host_conflict: destination changed: %s", name)
		}
		return os.Rename(temp, p.path)
	}
	if !os.IsNotExist(err) {
		return fmt.Errorf("host_conflict: destination appeared: %s", name)
	}
	// A hard link publishes a complete new file without replacing a concurrent
	// create. Both names are in the same directory and thus on one filesystem.
	if err := os.Link(temp, p.path); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("host_conflict: destination appeared: %s: %w", name, err)
		}
		return err
	}
	return nil
}
