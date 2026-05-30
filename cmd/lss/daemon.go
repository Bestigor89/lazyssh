package main

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
)

// maxScrollback is the maximum number of bytes of PTY output retained for
// replay when a client re-attaches to an existing session.
const maxScrollback = 256 * 1024

// scrollback is a simple capped byte buffer for PTY output.
type scrollback struct {
	data []byte
}

func (s *scrollback) write(p []byte) {
	s.data = append(s.data, p...)
	if len(s.data) > maxScrollback {
		s.data = s.data[len(s.data)-maxScrollback:]
	}
}

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

	// PTY drain: buffer output for replay on reattach and forward to the
	// current client (if any). All writes to the client conn happen under mu
	// so that the replay block in the accept loop cannot interleave with
	// live output.
	var mu sync.Mutex
	var current net.Conn
	var sb scrollback

	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				mu.Lock()
				sb.write(buf[:n])
				if current != nil {
					if werr := writeFrame(current, fData, buf[:n]); werr != nil {
						// Write failed — client is dead. Closing the conn
						// causes readFrame in handleClient to return an error,
						// which closes done, returns handleClient, and clears current.
						current.Close()
					}
				}
				mu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()

	// Accept loop runs continuously in a goroutine per connection so that
	// a busy session can immediately reject a second attach attempt.
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}

		go func(c net.Conn) {
			mu.Lock()
			busy := current != nil
			if !busy {
				// Replay buffered output before setting current so that the
				// PTY-drain goroutine cannot interleave live data between the
				// replay and the moment we start forwarding normally.
				if len(sb.data) > 0 {
					if werr := writeFrame(c, fData, sb.data); werr != nil {
						c.Close()
						mu.Unlock()
						return
					}
				}
				current = c
			}
			mu.Unlock()

			if busy {
				_ = writeFrame(c, fError, []byte("session already attached"))
				c.Close()
				return
			}

			handleClient(c, ptmx, shellDone)

			mu.Lock()
			current = nil
			mu.Unlock()
		}(conn)
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

					// TIOCSWINSZ only sends SIGWINCH when the size changes.
					// Set a dummy size first to guarantee a full redraw on
					// reattach even when the terminal dimensions are unchanged.
					_ = pty.Setsize(ptmx, &pty.Winsize{Rows: rows, Cols: cols + 1})
					_ = pty.Setsize(ptmx, &pty.Winsize{Rows: rows, Cols: cols})
				}
			case fDetach:
				return
			}
		}
	}()

	// Keepalive: detect dead clients when the shell produces no output.
	// Sends a zero-length fData frame every 30 s; a write failure means
	// the client is gone → close conn → reader goroutine returns → done closes.
	go func() {
		tick := time.NewTicker(30 * time.Second)
		defer tick.Stop()
		for {
			select {
			case <-tick.C:
				if err := writeFrame(conn, fData, nil); err != nil {
					conn.Close()
					return
				}
			case <-done:
				return
			case <-shellDone:
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
