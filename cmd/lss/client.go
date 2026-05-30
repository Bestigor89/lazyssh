package main

import (
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/term"
)

const detachKey = 0x1c // Ctrl-\

// runClient connects to a session socket and runs the interactive loop.
// Returns when the server closes the connection or the user detaches.
func runClient(socketPath string) error {
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return fmt.Errorf("cannot connect to session: %w", err)
	}
	defer conn.Close()

	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return fmt.Errorf("cannot set raw mode: %w", err)
	}
	defer term.Restore(fd, oldState)

	sendWinch(conn)

	// Forward SIGWINCH (terminal resize) to the remote PTY.
	sigwinch := make(chan os.Signal, 1)
	signal.Notify(sigwinch, syscall.SIGWINCH)
	defer signal.Stop(sigwinch)
	go func() {
		for range sigwinch {
			sendWinch(conn)
		}
	}()

	// Close the connection on SIGHUP / SIGTERM so the daemon detects the
	// disconnect immediately (e.g. when the SSH session is killed).
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGHUP, syscall.SIGTERM)
	defer signal.Stop(sigs)
	go func() {
		<-sigs
		conn.Close()
	}()

	var clientErr error

	// serverClosed is closed when the server side of the conn is done.
	serverClosed := make(chan struct{})

	// server → local stdout
	go func() {
		defer close(serverClosed)
		for {
			typ, payload, err := readFrame(conn)
			if err != nil {
				return
			}
			switch typ {
			case fData:
				if len(payload) > 0 {
					_, _ = os.Stdout.Write(payload)
				}
			case fError:
				// Daemon rejected connection (session busy, etc.).
				clientErr = fmt.Errorf("%s", payload)
				return
			}
		}
	}()

	// detached is closed when the user presses the detach key.
	detached := make(chan struct{})

	// local stdin → server (intercept detach key)
	// This goroutine may outlive runClient (blocked in os.Stdin.Read).
	// That is intentional: the process exits shortly after, taking it with it.
	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := os.Stdin.Read(buf)
			if err != nil {
				return
			}
			for i := 0; i < n; i++ {
				if buf[i] == detachKey {
					_ = writeFrame(conn, fDetach, nil)
					close(detached)
					return
				}
			}
			if err := writeFrame(conn, fData, buf[:n]); err != nil {
				return
			}
		}
	}()

	// Return as soon as the server closes OR the user detaches.
	// We do NOT wait for the stdin goroutine — it may be blocked in
	// os.Stdin.Read and would hang forever after the daemon dies.
	// The process exits shortly after runClient returns, cleaning it up.
	select {
	case <-serverClosed:
	case <-detached:
	}

	return clientErr
}

func sendWinch(conn net.Conn) {
	w, h, err := term.GetSize(int(os.Stdin.Fd())) // GetSize returns width, height
	if err != nil {
		return
	}
	payload := []byte{
		byte(h >> 8), byte(h), // rows = height
		byte(w >> 8), byte(w), // cols = width
	}
	_ = writeFrame(conn, fWinch, payload)
}
