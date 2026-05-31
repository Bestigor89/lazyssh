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

// session tracks all attached clients and the scrollback buffer.
// All writes to client conns go through session methods, serialised by mu,
// because writeFrame issues two Writes per frame and would otherwise interleave.
type session struct {
	mu      sync.Mutex
	clients map[net.Conn]struct{}
	sb      scrollback
}

// feed records PTY output and broadcasts it to every attached client.
func (s *session) feed(data []byte) {
	s.mu.Lock()
	s.sb.write(data)
	s.broadcastLocked(fData, data)
	s.mu.Unlock()
}

// ping broadcasts an empty fData frame so dead clients are detected and
// dropped even when the shell produces no output (global keepalive).
func (s *session) ping() {
	s.mu.Lock()
	s.broadcastLocked(fData, nil)
	s.mu.Unlock()
}

// broadcastLocked writes a frame to every client; caller must hold mu.
// A 5-second write deadline prevents one stalled mirror client from blocking
// the shared PTY (head-of-line blocking); a timed-out client is dropped.
func (s *session) broadcastLocked(typ byte, payload []byte) {
	for c := range s.clients {
		_ = c.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if err := writeFrame(c, typ, payload); err != nil {
			delete(s.clients, c)
			c.Close()
		} else {
			_ = c.SetWriteDeadline(time.Time{})
		}
	}
}

// add replays buffered scrollback to a new client, then registers it for
// future broadcasts. Replay runs under mu so live PTY output cannot interleave
// between the replay frame and normal forwarding.
// Returns false if the replay write fails; the conn is closed in that case.
func (s *session) add(c net.Conn) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.sb.data) > 0 {
		_ = c.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if err := writeFrame(c, fData, s.sb.data); err != nil {
			c.Close()
			return false
		}
		_ = c.SetWriteDeadline(time.Time{})
	}
	s.clients[c] = struct{}{}
	return true
}

// remove deregisters a client and closes its conn.
func (s *session) remove(c net.Conn) {
	s.mu.Lock()
	delete(s.clients, c)
	s.mu.Unlock()
	c.Close()
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

	sess := &session{clients: make(map[net.Conn]struct{})}

	// Close listener and remove files when shell exits.
	shellDone := make(chan struct{})
	go func() {
		cmd.Wait()
		close(shellDone)
		ln.Close()
		os.Remove(socketPath)
		os.Remove(pidFile)
	}()

	// PTY drain: buffer output for replay and broadcast to all attached clients.
	go func() {
		buf := make([]byte, 4096)
		for {
			n, readErr := ptmx.Read(buf)
			if n > 0 {
				sess.feed(buf[:n])
			}
			if readErr != nil {
				return
			}
		}
	}()

	// Global keepalive: detect dead clients even when the shell produces no
	// output. Replaces the old per-client keepalive goroutine.
	go func() {
		tick := time.NewTicker(30 * time.Second)
		defer tick.Stop()
		for {
			select {
			case <-tick.C:
				sess.ping()
			case <-shellDone:
				return
			}
		}
	}()

	// Accept loop: each incoming connection is a new mirror client.
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}

		go func(c net.Conn) {
			if !sess.add(c) {
				return
			}
			handleClient(c, ptmx, shellDone)
			sess.remove(c)
		}(conn)
	}
}

// handleClient bridges a connected client to the PTY until detach or shell exit.
// Closing conn is the caller's responsibility (sess.remove).
func handleClient(conn net.Conn, ptmx *os.File, shellDone <-chan struct{}) {
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

	select {
	case <-done:
	case <-shellDone:
		// Shell exited; conn will be closed by sess.remove in the accept loop.
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
