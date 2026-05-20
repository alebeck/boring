package ssh_config

import (
	"testing"

	"github.com/alebeck/boring/internal/auth"
)

const (
	encKeyPath   = "../../test/testdata/keys/client_enc"
	plainKeyPath = "../../test/testdata/keys/client"
)

func TestLoadPrivateKeyWithPassphrase(t *testing.T) {
	p := auth.FuncPrompter(func(_, _ string, _ []string, _ []bool) ([]string, error) {
		return []string{"testpass"}, nil
	})
	signer, err := loadPrivateKeyInteractive(encKeyPath, p)
	if err != nil {
		t.Fatalf("load encrypted key: %v", err)
	}
	if signer == nil {
		t.Fatal("nil signer")
	}
}

func TestLoadPrivateKeyWrongPassphrase(t *testing.T) {
	p := auth.FuncPrompter(func(_, _ string, _ []string, _ []bool) ([]string, error) {
		return []string{"wrong"}, nil
	})
	if _, err := loadPrivateKeyInteractive(encKeyPath, p); err == nil {
		t.Fatal("expected error for wrong passphrase")
	}
}

func TestLoadPrivateKeyEncryptedNoPrompter(t *testing.T) {
	if _, err := loadPrivateKeyInteractive(encKeyPath, nil); err == nil {
		t.Fatal("expected error: encrypted key with nil prompter")
	}
}

func TestLoadPrivateKeyUnencryptedNoPrompter(t *testing.T) {
	signer, err := loadPrivateKeyInteractive(plainKeyPath, nil)
	if err != nil || signer == nil {
		t.Fatalf("unencrypted key should load without a prompter: signer=%v err=%v", signer, err)
	}
}
