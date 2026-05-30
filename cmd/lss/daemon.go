package main

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"

	"github.com/creack/pty"
)

// runDaemon is the daemon mode: allocate PTY, start shell, serve the unix socket.
// Called when LSS_DAEMON=1 is set in the environment.
func runDaemon(name, socketPath string) {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}

	cmd := exec.Command(shell, "-l")
	cmd.Env = append(os.Environ(), "TERM=xterm-256color", "LSS_SESSION="+name)

	ptmx, err := pty.Start(cmd)
	if err != nil {
		os.Exit(1)
	}
	defer ptmx.Close()

	// Ignore SIGHUP — daemon must survive SSH disconnect.
	signal.Ignore(syscall.SIGHUP)

	// Create unix socket.
	os.Remove(socketPath)
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		cmd.Process.Kill()
		os.Exit(1)
	}

	// Write PID file so `lss list` can check liveness.
	pidFile := socketPath[:len(socketPath)-5] + ".pid"
	writePID(pidFile)

	// Close listener and remove files when shell exits.
	shellDone := make(chan struct{})
	go func() {
		cmd.Wait()
		close(shellDone)
		ln.Close()
		os.Remove(socketPath)
		os.Remove(pidFile)
	}()

	// PTY drain: forward output to the current client (if any).
	var mu sync.Mutex
	var current net.Conn

	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				mu.Lock()
				c := current
				mu.Unlock()
				if c != nil {
					_ = writeFrame(c, fData, buf[:n])
				}
			}
			if err != nil {
				return
			}
		}
	}()

	// Accept clients one at a time.
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}

		mu.Lock()
		current = conn
		mu.Unlock()

		handleClient(conn, ptmx, shellDone)

		mu.Lock()
		current = nil
		mu.Unlock()
	}
}

// handleClient bridges a connected client to the PTY until detach or shell exit.
func handleClient(conn net.Conn, ptmx *os.File, shellDone <-chan struct{}) {
	defer conn.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			typ, payload, err := readFrame(conn)
			if err != nil {
				return
			}
			switch typ {
			case fData:
				_, _ = ptmx.Write(payload)
			case fWinch:
				if len(payload) == 4 {
					rows := uint16(payload[0])<<8 | uint16(payload[1])
					cols := uint16(payload[2])<<8 | uint16(payload[3])
					_ = pty.Setsize(ptmx, &pty.Winsize{Rows: rows, Cols: cols})
				}
			case fDetach:
				return
			}
		}
	}()

	select {
	case <-done:
	case <-shellDone:
	}
}

func writePID(path string) {
	f, err := os.Create(path)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%d\n", os.Getpid())
}
