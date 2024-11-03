package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/alebeck/boring/internal/config"
	"github.com/alebeck/boring/internal/log"
)

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
		f.Close()
		log.Infof("Hi! Created boring config file: %s", config.Path)
	}
	return nil
}
