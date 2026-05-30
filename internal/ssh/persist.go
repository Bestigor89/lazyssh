package ssh

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"strings"
)

const lssRelPath = ".lazyssh/bin/lss"

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

// HelperInstalled reports whether the lss binary exists on the remote host.
func (c *Client) HelperInstalled(home string) bool {
	_, err := c.sftpClient.Stat(path.Join(home, lssRelPath))
	return err == nil
}

// UploadHelper uploads the lss binary to ~/.lazyssh/bin/lss on the remote.
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
	return c.sftpClient.Chmod(dest, 0755)
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
