//go:build windows

// Package secrets stores secret values only in Windows Credential Manager.
// The SQLite database holds references (target names), never secret values.
package secrets

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/danieljoos/wincred"
	"golang.org/x/term"
)

const targetPrefix = "opentrinity/"

var ErrNotFound = errors.New("secret not found in Windows Credential Manager")

// GatewayTarget is the credential target for the local gateway bearer key.
func GatewayTarget() string { return targetPrefix + "gateway/key" }

// LiteLLMKeyTarget is the credential target for the private LiteLLM master key.
func LiteLLMKeyTarget() string { return targetPrefix + "litellm/key" }

// ProviderTarget maps a credential row to a stable Credential Manager target.
func ProviderTarget(providerID, credentialID int64) string {
	return fmt.Sprintf("%sprovider/%d/credential/%d", targetPrefix, providerID, credentialID)
}

// CredentialTarget maps the secret_ref stored in SQLite to its Credential
// Manager target. The reference itself is safe to persist; the value is not.
func CredentialTarget(ref string) string { return targetPrefix + "secret/" + ref }

// Set writes a secret to Windows Credential Manager.
func Set(target, value string) error {
	if !strings.HasPrefix(target, targetPrefix) {
		return errors.New("invalid secret target")
	}
	if value == "" {
		return errors.New("secret value is empty")
	}
	c := wincred.NewGenericCredential(target)
	c.CredentialBlob = []byte(value)
	c.Persist = wincred.PersistLocalMachine
	c.Comment = "OpenTrinity local secret; managed by the opentrinity CLI"
	if err := c.Write(); err != nil {
		return err
	}
	c.CredentialBlob = nil
	return nil
}

// Get reads a secret from Windows Credential Manager.
func Get(target string) (string, error) {
	c, err := wincred.GetGenericCredential(target)
	if err != nil {
		if errors.Is(err, wincred.ErrElementNotFound) {
			return "", ErrNotFound
		}
		return "", err
	}
	v := string(c.CredentialBlob)
	c.CredentialBlob = nil
	return v, nil
}

// Delete removes a secret from Windows Credential Manager.
func Delete(target string) error {
	c, err := wincred.GetGenericCredential(target)
	if err != nil {
		if errors.Is(err, wincred.ErrElementNotFound) {
			return nil
		}
		return err
	}
	return c.Delete()
}

// NewGatewayKey creates a high-entropy local bearer key.
func NewGatewayKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// ReadMasked reads a secret from the terminal with echo disabled. It is used
// by CLI commands; the TUI has its own masked input component.
func ReadMasked(prompt string) (string, error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return "", errors.New("secret entry requires an interactive terminal; use `opentrinity tui` or pipe via a protected local input path")
	}
	_, _ = fmt.Fprint(os.Stderr, prompt+": ")
	b, err := term.ReadPassword(int(os.Stdin.Fd()))
	_, _ = fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}
