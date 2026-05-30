package main

import (
	"fmt"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"golang.org/x/term"
)

const detachKey = 0x1c // Ctrl-\

// runClient connects to a session socket and runs the interactive loop.
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

	// Send initial window size — triggers a forced redraw on the daemon side.
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

	var wg sync.WaitGroup
	var clientErr error

	// server → local stdout
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			typ, payload, err := readFrame(conn)
			if err != nil {
				return
			}
			switch typ {
			case fData:
				_, _ = os.Stdout.Write(payload)
			case fError:
				// Bug 2 fix: daemon rejected us (session busy). Print the
				// message after restoring the terminal so it renders correctly.
				clientErr = fmt.Errorf("%s", payload)
				conn.Close()
				return
			}
		}
	}()

	// local stdin → server (intercept detach key)
	wg.Add(1)
	go func() {
		defer wg.Done()
		buf := make([]byte, 1024)
		for {
			n, err := os.Stdin.Read(buf)
			if err != nil {
				return
			}
			for i := 0; i < n; i++ {
				if buf[i] == detachKey {
					_ = writeFrame(conn, fDetach, nil)
					conn.Close()
					return
				}
			}
			if err := writeFrame(conn, fData, buf[:n]); err != nil {
				return
			}
		}
	}()

	wg.Wait()
	return clientErr
}

func sendWinch(conn net.Conn) {
	h, w, err := term.GetSize(int(os.Stdin.Fd()))
	if err != nil {
		return
	}
	payload := []byte{
		byte(h >> 8), byte(h),
		byte(w >> 8), byte(w),
	}
	_ = writeFrame(conn, fWinch, payload)
}
