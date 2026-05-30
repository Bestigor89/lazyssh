package ssh

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
)

const (
	lssRelPath  = ".lazyssh/bin/lss"
	lssHashFile = ".lazyssh/bin/lss.sha256"
)

// Session represents a running lss session on a remote host.
type Session struct {
	Name string `json:"name"`
}

// RemoteHome returns the remote user's home directory.
func (c *Client) RemoteHome() (string, error) {
	out, err := c.RunCommand("echo $HOME")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// DetectArch returns "amd64" or "arm64" for the remote host, or an error for
// unsupported or non-Linux systems.
func DetectArch(c *Client) (string, error) {
	out, err := c.RunCommand("uname -sm")
	if err != nil {
		return "", err
	}
	s := strings.ToLower(strings.TrimSpace(string(out)))
	if !strings.HasPrefix(s, "linux") {
		return "", fmt.Errorf("not a Linux host: %s", strings.TrimSpace(string(out)))
	}
	switch {
	case strings.Contains(s, "x86_64"):
		return "amd64", nil
	case strings.Contains(s, "aarch64"), strings.Contains(s, "arm64"):
		return "arm64", nil
	default:
		return "", fmt.Errorf("unsupported arch: %s", strings.TrimSpace(string(out)))
	}
}

// HelperReady reports whether the lss binary on the server exists AND matches
// the embedded binary. Returns false when lss is absent or outdated (bug 3 fix).
// expected may be nil — in that case only the existence check is performed.
func (c *Client) HelperReady(home string, expected []byte) bool {
	if _, err := c.sftpClient.Stat(path.Join(home, lssRelPath)); err != nil {
		return false // not installed at all
	}
	if len(expected) == 0 {
		return true // no embedded binary to compare against
	}

	// Read the hash written alongside the binary during deployment.
	f, err := c.sftpClient.Open(path.Join(home, lssHashFile))
	if err != nil {
		return false // hash file missing → treat as outdated
	}
	defer f.Close()
	stored, _ := io.ReadAll(f)

	h := sha256.Sum256(expected)
	return strings.TrimSpace(string(stored)) == hex.EncodeToString(h[:])
}

// UploadHelper uploads the lss binary to ~/.lazyssh/bin/lss and writes a
// SHA-256 hash file used by HelperReady for future version checks.
func (c *Client) UploadHelper(home string, data []byte) error {
	binDir := path.Join(home, ".lazyssh", "bin")
	if err := c.sftpClient.MkdirAll(binDir); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}

	dest := path.Join(home, lssRelPath)
	f, err := c.sftpClient.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC)
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	if err := c.sftpClient.Chmod(dest, 0755); err != nil {
		return fmt.Errorf("chmod: %w", err)
	}

	// Write hash file so future connects can detect a version mismatch.
	h := sha256.Sum256(data)
	hashPath := path.Join(home, lssHashFile)
	hf, err := c.sftpClient.OpenFile(hashPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC)
	if err != nil {
		return nil // non-fatal: update check will just redeploy next time
	}
	defer hf.Close()
	_, _ = hf.Write([]byte(hex.EncodeToString(h[:])))
	return nil
}

// ListSessions returns the active lss sessions on the remote host.
func ListSessions(c *Client, home string) ([]Session, error) {
	lss := path.Join(home, lssRelPath)
	out, err := c.RunCommand(lss + " list")
	if err != nil {
		return nil, fmt.Errorf("lss list: %w", err)
	}
	var sessions []Session
	if err := json.Unmarshal(out, &sessions); err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	if sessions == nil {
		sessions = []Session{}
	}
	return sessions, nil
}

// NewSessionCmd returns the remote command to create and attach to a new session.
func NewSessionCmd(home, name string) string {
	return path.Join(home, lssRelPath) + " new " + shellQuote(name)
}

// AttachSessionCmd returns the remote command to attach to an existing session.
func AttachSessionCmd(home, name string) string {
	return path.Join(home, lssRelPath) + " attach " + shellQuote(name)
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
