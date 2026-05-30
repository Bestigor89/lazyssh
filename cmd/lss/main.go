// lss is the lazyssh session helper that runs on remote servers.
// It manages persistent terminal sessions that survive SSH disconnects.
//
// Usage:
//
//	lss new <name>     — create and attach to a new session
//	lss attach <name>  — attach to an existing session
//	lss list           — list sessions as JSON
//	lss kill <name>    — terminate a session
//
// Detach key: Ctrl-\
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

func sessionsDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".lazyssh", "sessions")
}

func sockPath(name string) string { return filepath.Join(sessionsDir(), name+".sock") }
func pidPath(name string) string  { return filepath.Join(sessionsDir(), name+".pid") }

func main() {
	// Internal: daemon re-exec. Invoked as: LSS_DAEMON=1 lss <name> <sockPath>
	if os.Getenv("LSS_DAEMON") == "1" && len(os.Args) == 3 {
		runDaemon(os.Args[1], os.Args[2])
		return
	}

	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "new":
		requireArg("lss new <name>")
		cmdNew(os.Args[2])
	case "attach", "a":
		requireArg("lss attach <name>")
		cmdAttach(os.Args[2])
	case "list", "ls":
		cmdList()
	case "kill":
		requireArg("lss kill <name>")
		cmdKill(os.Args[2])
	default:
		usage()
		os.Exit(1)
	}
}

func requireArg(hint string) {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage:", hint)
		os.Exit(1)
	}
}

// cmdNew starts a new daemon and immediately attaches to it.
func cmdNew(name string) {
	sp := sockPath(name)
	if _, err := os.Stat(sp); err == nil {
		fatalf("session %q already exists; use 'lss attach %s'", name, name)
	}
	if err := os.MkdirAll(sessionsDir(), 0700); err != nil {
		fatalf("mkdir: %v", err)
	}

	exe, err := os.Executable()
	if err != nil {
		fatalf("executable: %v", err)
	}
	devNull, _ := os.Open(os.DevNull)
	defer devNull.Close()

	// Re-exec self as a background daemon.
	cmd := exec.Command(exe, name, sp)
	cmd.Env = append(os.Environ(), "LSS_DAEMON=1")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = devNull, devNull, devNull
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		fatalf("start daemon: %v", err)
	}

	// Wait up to 2 s for the socket to appear.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(sp); err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if _, err := os.Stat(sp); err != nil {
		fatalf("daemon did not start in time")
	}

	if err := runClient(sp); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}

func cmdAttach(name string) {
	if err := runClient(sockPath(name)); err != nil {
		fatalf("%v", err)
	}
}

// Session holds info about one lss session for the JSON output.
type Session struct {
	Name string `json:"name"`
}

func cmdList() {
	dir := sessionsDir()
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		fmt.Println("[]")
		return
	}
	if err != nil {
		fatalf("readdir: %v", err)
	}

	sessions := make([]Session, 0)
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".pid") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".pid")
		pp := filepath.Join(dir, e.Name())

		data, err := os.ReadFile(pp)
		if err != nil {
			continue
		}
		var pid int
		fmt.Sscanf(strings.TrimSpace(string(data)), "%d", &pid)
		if pid <= 0 {
			continue
		}
		if err := syscall.Kill(pid, 0); err != nil {
			// Stale entry — clean up.
			os.Remove(pp)
			os.Remove(sockPath(name))
			continue
		}
		sessions = append(sessions, Session{Name: name})
	}

	_ = json.NewEncoder(os.Stdout).Encode(sessions)
}

func cmdKill(name string) {
	data, err := os.ReadFile(pidPath(name))
	if err != nil {
		fatalf("session %q not found", name)
	}
	var pid int
	fmt.Sscanf(strings.TrimSpace(string(data)), "%d", &pid)
	if pid > 0 {
		_ = syscall.Kill(pid, syscall.SIGTERM)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "lss — lazyssh session helper")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "usage: lss <command> [name]")
	fmt.Fprintln(os.Stderr, "  new <name>     create and attach to a new session")
	fmt.Fprintln(os.Stderr, "  attach <name>  attach to an existing session")
	fmt.Fprintln(os.Stderr, "  list           list sessions (JSON)")
	fmt.Fprintln(os.Stderr, "  kill <name>    terminate a session")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "detach: Ctrl-\\")
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "lss: "+format+"\n", args...)
	os.Exit(1)
}
