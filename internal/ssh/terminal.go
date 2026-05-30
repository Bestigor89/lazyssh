package ssh

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"

	"github.com/rivo/tview"

	"github.com/Bestigor89/lazyssh/internal/model"
)

// LaunchTerminal suspends the TUI, opens an interactive SSH session using the
// system ssh binary, then resumes the TUI when the session ends.
//
// remoteCmd is an optional command to run on the remote host (e.g. a lss
// invocation). An empty string opens a plain login shell. When remoteCmd is
// non-empty, -t is added to force PTY allocation.
func LaunchTerminal(tApp *tview.Application, host *model.Host, remoteCmd string) error {
	var runErr error

	tApp.Suspend(func() {
		args := sshArgs(host, remoteCmd)
		cmd := exec.Command("ssh", args...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			// exec.ExitError is normal (user typed "exit", detached, etc.).
			if _, ok := err.(*exec.ExitError); !ok {
				runErr = fmt.Errorf("ssh: %w", err)
			}
		}
	})

	return runErr
}

// sshArgs builds the argument list for the ssh command.
// When remoteCmd is non-empty it is appended after the host and -t is added.
func sshArgs(host *model.Host, remoteCmd string) []string {
	args := []string{"-p", strconv.Itoa(host.EffectivePort())}
	if host.AuthType == model.AuthTypeKey && host.KeyPath != "" {
		args = append(args, "-i", host.KeyPath)
	}
	if remoteCmd != "" {
		args = append(args, "-t", host.UserHost(), remoteCmd)
	} else {
		args = append(args, host.UserHost())
	}
	return args
}
