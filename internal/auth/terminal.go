package auth

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

// TerminalPrompter reads auth answers from a terminal. Hidden questions
// (echo=false) require a real TTY; echoed questions work on any reader.
type TerminalPrompter struct {
	in  io.Reader // defaults to os.Stdin
	out io.Writer // defaults to os.Stderr
}

// Compile-time check that *TerminalPrompter satisfies Prompter.
var _ Prompter = (*TerminalPrompter)(nil)

// NewTerminalPrompter returns a prompter bound to stdin/stderr.
func NewTerminalPrompter() *TerminalPrompter {
	return &TerminalPrompter{in: os.Stdin, out: os.Stderr}
}

func (p *TerminalPrompter) Prompt(name, instruction string,
	questions []string, echo []bool) ([]string, error) {
	if instruction != "" {
		fmt.Fprintln(p.out, instruction)
	}
	answers := make([]string, len(questions))
	for i, q := range questions {
		fmt.Fprint(p.out, q)
		var (
			ans string
			err error
		)
		if i < len(echo) && echo[i] {
			ans, err = bufio.NewReader(p.in).ReadString('\n')
			ans = strings.TrimRight(ans, "\r\n")
		} else {
			ans, err = readHidden(p.in, p.out)
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read answer: %w", err)
		}
		answers[i] = ans
	}
	return answers, nil
}

// readHidden reads a line without echoing it, requiring a TTY.
func readHidden(in io.Reader, out io.Writer) (string, error) {
	f, ok := in.(*os.File)
	if !ok || !term.IsTerminal(int(f.Fd())) {
		return "", fmt.Errorf("hidden input requires a terminal")
	}
	b, err := term.ReadPassword(int(f.Fd()))
	fmt.Fprintln(out)
	return string(b), err
}
