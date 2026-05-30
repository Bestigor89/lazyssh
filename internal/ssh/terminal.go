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
func LaunchTerminal(tApp *tview.Application, host *model.Host) error {
	var runErr error

	tApp.Suspend(func() {
		args := sshArgs(host)
		cmd := exec.Command("ssh", args...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			// exec.ExitError is normal (user typed "exit" or closed session).
			if _, ok := err.(*exec.ExitError); !ok {
				runErr = fmt.Errorf("ssh: %w", err)
			}
		}
	})

	return runErr
}

// sshArgs builds the argument list for the ssh command.
func sshArgs(host *model.Host) []string {
	args := []string{
		"-p", strconv.Itoa(host.EffectivePort()),
	}
	if host.AuthType == model.AuthTypeKey && host.KeyPath != "" {
		args = append(args, "-i", host.KeyPath)
	}
	args = append(args, host.UserHost())
	return args
}
