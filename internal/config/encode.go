package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/alebeck/boring/internal/tunnel"
)

const backupSuffix = ".bak"

// Save validates cfg and writes it to path as TOML. On the first save it
// preserves the existing file (with any hand-written comments) as path+".bak".
// The write is atomic: a temp file is renamed over path, so a crash never
// leaves a half-written config. An existing file's permissions are preserved;
// a freshly created config is written 0600.
//
// Save never mutates cfg or its tunnels; it builds an independent encoding
// representation (encodeConfig) before writing.
func Save(cfg *Config, path string) error {
	if err := Validate(cfg.Tunnels); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}
	if err := backupOnce(path); err != nil {
		return err
	}
	return atomicWrite(path, toEncodeConfig(cfg))
}

// encodeConfig is the TOML encoding view of a Config. It exists so Save can
// write each tunnel in exactly one form (legacy shorthand or
// [[tunnels.forward]] blocks) without mutating the caller's Config.
type encodeConfig struct {
	Tunnels   []encodeDesc `toml:"tunnels"`
	KeepAlive *int         `toml:"keep_alive,omitempty"`
}

// encodeDesc is the TOML encoding view of a tunnel.Desc.
//
// Unlike Desc, every shorthand field here uses a type whose omitempty works:
// LocalAddress/RemoteAddress are plain strings and Mode is a *string, so a
// multi-forward tunnel can clear all three and have none of them written.
// (Desc's local/remote lack omitempty and Desc.Mode is a numeric type whose
// MarshalTOML still emits on the zero value, so Desc cannot be encoded
// directly without writing a stray tunnel-level local/remote/mode.)
type encodeDesc struct {
	Name          string           `toml:"name"`
	LocalAddress  string           `toml:"local,omitempty"`
	RemoteAddress string           `toml:"remote,omitempty"`
	Mode          *string          `toml:"mode,omitempty"`
	Host          string           `toml:"host"`
	User          string           `toml:"user,omitempty"`
	IdentityFile  string           `toml:"identity,omitempty"`
	Port          *int             `toml:"port,omitempty"`
	KeepAlive     *int             `toml:"keep_alive,omitempty"`
	Group         string           `toml:"group,omitempty"`
	Forwards      []tunnel.Forward `toml:"forward,omitempty"`
}

// toEncodeConfig builds the encoding view of cfg without mutating it.
func toEncodeConfig(cfg *Config) *encodeConfig {
	tunnels := make([]encodeDesc, len(cfg.Tunnels))
	for i := range cfg.Tunnels {
		tunnels[i] = toEncodeDesc(&cfg.Tunnels[i])
	}
	return &encodeConfig{Tunnels: tunnels, KeepAlive: cfg.KeepAlive}
}

// toEncodeDesc builds the encoding view of a single tunnel.
//
// A tunnel is written in exactly one form:
//   - exactly one forward (or none — a directly built Desc that only set the
//     legacy fields) → the legacy local/remote/mode shorthand at the tunnel
//     level, no [[tunnels.forward]] block;
//   - more than one forward → [[tunnels.forward]] blocks, no tunnel-level
//     local/remote/mode.
//
// Writing exactly one form is required: a tunnel carrying both the shorthand
// and forward blocks would fail to reload (config.Load rejects that mix).
func toEncodeDesc(d *tunnel.Desc) encodeDesc {
	e := encodeDesc{
		Name:         d.Name,
		Host:         d.Host,
		User:         d.User,
		IdentityFile: d.IdentityFile,
		Port:         d.Port,
		KeepAlive:    d.KeepAlive,
		Group:        d.Group,
	}
	if len(d.Forwards) > 1 {
		e.Forwards = d.Forwards
		return e
	}
	// Single-forward (or legacy-only) tunnel: write the shorthand. Prefer the
	// normalized Forwards[0] when present, falling back to the legacy fields.
	f := shorthandForward(d)
	e.LocalAddress = string(f.LocalAddress)
	e.RemoteAddress = string(f.RemoteAddress)
	// Only write a tunnel-level mode when it is not the Local default: a missing
	// mode key reloads as tunnel.Local, so omitting it avoids redundant churn in
	// hand-written configs while staying round-trip correct.
	if f.Mode != tunnel.Local {
		mode := f.Mode.ConfigValue()
		e.Mode = &mode
	}
	return e
}

// shorthandForward returns the single forward a tunnel should be encoded as
// the legacy shorthand. It uses Forwards[0] when the tunnel has been
// normalized, and otherwise the tunnel's own legacy local/remote/mode fields
// (the shape of a Desc built directly, e.g. in tests, without going through
// config.Load).
func shorthandForward(d *tunnel.Desc) tunnel.Forward {
	if len(d.Forwards) == 1 {
		return d.Forwards[0]
	}
	return tunnel.Forward{
		LocalAddress:  d.LocalAddress,
		RemoteAddress: d.RemoteAddress,
		Mode:          d.Mode,
	}
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
func atomicWrite(path string, cfg *encodeConfig) error {
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
