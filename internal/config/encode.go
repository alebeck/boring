package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

const backupSuffix = ".bak"

// Save validates cfg and writes it to path as TOML. On the first save it
// preserves the existing file (with any hand-written comments) as path+".bak".
// The write is atomic: a temp file is renamed over path, so a crash never
// leaves a half-written config. An existing file's permissions are preserved;
// a freshly created config is written 0600.
func Save(cfg *Config, path string) error {
	if err := Validate(cfg.Tunnels); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}
	if err := backupOnce(path); err != nil {
		return err
	}
	return atomicWrite(path, cfg)
}

// backupOnce copies path to path+".bak" if that backup does not yet exist and
// the original file is present. The backup is the pristine original, kept
// forever — later saves never overwrite it.
func backupOnce(path string) error {
	bak := path + backupSuffix
	if _, err := os.Stat(bak); err == nil {
		return nil // backup already exists, never overwrite it
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil // nothing to back up (fresh config)
	}
	if err != nil {
		return fmt.Errorf("failed to read config for backup: %w", err)
	}
	if err := os.WriteFile(bak, data, 0600); err != nil {
		return fmt.Errorf("failed to write backup: %w", err)
	}
	return nil
}

// atomicWrite encodes cfg into a temp file in path's directory, fsyncs it, and
// renames it over path.
func atomicWrite(path string, cfg *Config) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".boring-*.toml")
	if err != nil {
		return fmt.Errorf("failed to create temp config: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op after a successful rename

	// Preserve the existing file's permissions; a fresh config keeps the temp
	// file's 0600 default, a sensible private default for a boring config.
	if info, statErr := os.Stat(path); statErr == nil {
		if err := tmp.Chmod(info.Mode().Perm()); err != nil {
			tmp.Close()
			return fmt.Errorf("failed to set config permissions: %w", err)
		}
	}

	if err := toml.NewEncoder(tmp).Encode(cfg); err != nil {
		tmp.Close()
		return fmt.Errorf("failed to encode config: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("failed to sync config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("failed to close temp config: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("failed to install config: %w", err)
	}
	return nil
}
