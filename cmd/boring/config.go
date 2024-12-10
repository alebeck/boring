package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/alebeck/boring/internal/config"
	"github.com/alebeck/boring/internal/log"
)

const defaultConfig = `# boring config file
# An example (local) tunnel is defined below.
# For more examples, please visit the project's GitHub page.
# All lines starting with '#' are comments.

#[[tunnels]]
#name = "dev"  # Name for the tunnel
#local = "localhost:9000"  # Local address to listen on
#remote = "localhost:9000"  # Remote address to forward to
#host = "dev-server"  # Hostname of the server, tries to match against ssh config
#port = 22  # (Optional) Server port, defaults to 22
#user = "joe"  # (Optional) Username, tries ssh config and defaults to $USER
#identity = "~/.ssh/id_dev"  # (Optional) Key file, tries ssh config and defaults to default keys

`

func openConfig() {
	if err := ensureConfig(); err != nil {
		log.Fatalf("could not create config file: %v", err)
	}

	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
		if runtime.GOOS == "windows" {
			editor = "notepad"
		}
	}

	cmd := exec.Command(editor, config.Path)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Run()
}

// Checks if config file exists, otherwise creates it
func ensureConfig() error {
	if _, statErr := os.Stat(config.Path); statErr != nil {
		d := filepath.Dir(config.Path)
		if err := os.MkdirAll(d, 0700); err != nil {
			return err
		}
		f, err := os.OpenFile(config.Path, os.O_RDWR|os.O_CREATE, 0600)
		if err != nil {
			return err
		}
		defer f.Close()
		if _, err := f.WriteString(defaultConfig); err != nil {
			return err
		}
		log.Infof("Hi! Created boring config file: %s", config.Path)
	}
	return nil
}
