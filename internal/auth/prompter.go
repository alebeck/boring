// Package auth provides interactive SSH authentication prompting.
package auth

import "fmt"

// Prompter asks the user for interactive authentication input: SSH
// keyboard-interactive challenges (2FA codes) and private-key passphrases.
type Prompter interface {
	// Prompt presents one or more questions and returns one answer each.
	// echo[i] reports whether question i's input may be shown on screen;
	// echo must be the same length as questions.
	Prompt(name, instruction string, questions []string, echo []bool) ([]string, error)
}

// FuncPrompter adapts a plain function to the Prompter interface.
type FuncPrompter func(name, instruction string, questions []string, echo []bool) ([]string, error)

func (f FuncPrompter) Prompt(name, instruction string, questions []string, echo []bool) ([]string, error) {
	return f(name, instruction, questions, echo)
}

// PassphrasePromptName is the prompt name used for private-key passphrase
// requests. It distinguishes them from SSH keyboard-interactive (2FA)
// challenges, which carry the server-supplied name.
const PassphrasePromptName = "passphrase"

// Passphrase is a convenience helper: a single hidden question.
func Passphrase(p Prompter, keyPath string) (string, error) {
	ans, err := p.Prompt(PassphrasePromptName, "",
		[]string{"Enter passphrase for " + keyPath + ": "}, []bool{false})
	if err != nil {
		return "", fmt.Errorf("failed to prompt passphrase for %s: %w", keyPath, err)
	}
	if len(ans) != 1 {
		return "", ErrAborted
	}
	return ans[0], nil
}
