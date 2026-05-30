package ssh

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
	"github.com/pkg/sftp"

	"github.com/Bestigor89/lazyssh/internal/model"
)

// FileInfo describes a single entry in a directory listing.
type FileInfo struct {
	Name    string
	IsDir   bool
	Size    int64
	Mode    os.FileMode
	ModTime time.Time
}

// Client wraps an active SFTP session.
type Client struct {
	sshConn    *gossh.Client
	sftpClient *sftp.Client
	agentConn  io.Closer // SSH agent socket; may be nil
}

// Connect establishes an SSH+SFTP connection to host.
//
// password is the decrypted plaintext password (may be empty when using key
// auth). onUnknownHost is invoked with the hostname and SHA-256 fingerprint
// when the server's key is not in known_hosts; return true to trust and save,
// false to abort. It may be nil (treats all unknown hosts as rejected).
func Connect(
	host *model.Host,
	password string,
	onUnknownHost func(hostname, fingerprint string) bool,
) (*Client, error) {
	addr := host.Addr()

	authMethods, agentConn, err := buildAuth(host, password)
	if err != nil {
		return nil, err
	}

	knownHostsPath := filepath.Join(os.Getenv("HOME"), ".ssh", "known_hosts")

	// Load known_hosts once, tolerating any malformed lines.
	db, err := loadTolerantKnownHosts(knownHostsPath)
	if err != nil {
		return nil, fmt.Errorf("load known_hosts: %w", err)
	}

	var (
		capturedKey      gossh.PublicKey
		capturedHostname string
		hostUnknown      bool
	)

	strictCallback := func(hostname string, remote net.Addr, key gossh.PublicKey) error {
		capturedKey = key
		capturedHostname = hostname

		if db == nil {
			// known_hosts file doesn't exist yet — TOFU (trust on first use).
			hostUnknown = true
			return fmt.Errorf("host not verified")
		}

		err := db(hostname, remote, key)
		if err == nil {
			return nil // known and trusted
		}

		var keyErr *knownhosts.KeyError
		if errors.As(err, &keyErr) {
			if len(keyErr.Want) == 0 {
				// Host not in known_hosts.
				hostUnknown = true
				return fmt.Errorf("host not verified")
			}
			// Key mismatch — hard fail (possible MITM).
			return fmt.Errorf("HOST KEY CHANGED for %s — possible MITM attack!\n"+
				"Remove the old entry from ~/.ssh/known_hosts to reconnect.", hostname)
		}

		// Any other verification error: treat as unknown.
		hostUnknown = true
		return fmt.Errorf("host not verified")
	}

	sshConfig := &gossh.ClientConfig{
		User:            host.User,
		Auth:            authMethods,
		HostKeyCallback: strictCallback,
		Timeout:         30 * time.Second,
	}

	sshConn, err := gossh.Dial("tcp", addr, sshConfig)
	if err != nil && hostUnknown {
		// Ask the caller whether to trust this host.
		fingerprint := ""
		if capturedKey != nil {
			fingerprint = gossh.FingerprintSHA256(capturedKey)
		}
		if onUnknownHost == nil || !onUnknownHost(capturedHostname, fingerprint) {
			return nil, fmt.Errorf("host key not trusted")
		}

		// User trusted — persist the key.
		if addErr := appendKnownHost(knownHostsPath, capturedHostname, capturedKey); addErr != nil {
			return nil, fmt.Errorf("save known_hosts: %w", addErr)
		}

		// Retry with the updated file.
		db2, err2 := loadTolerantKnownHosts(knownHostsPath)
		if err2 != nil {
			return nil, fmt.Errorf("reload known_hosts: %w", err2)
		}
		if db2 == nil {
			return nil, fmt.Errorf("known_hosts is empty after writing key")
		}
		sshConfig.HostKeyCallback = db2
		sshConn, err = gossh.Dial("tcp", addr, sshConfig)
	}
	if err != nil {
		return nil, fmt.Errorf("ssh connect: %w", err)
	}

	sftpClient, err := sftp.NewClient(sshConn)
	if err != nil {
		sshConn.Close()
		return nil, fmt.Errorf("sftp: %w", err)
	}

	return &Client{sshConn: sshConn, sftpClient: sftpClient, agentConn: agentConn}, nil
}

// Close shuts down the SFTP session and SSH connection.
func (c *Client) Close() {
	if c.sftpClient != nil {
		c.sftpClient.Close()
	}
	if c.sshConn != nil {
		c.sshConn.Close()
	}
	if c.agentConn != nil {
		c.agentConn.Close()
	}
}

// HomeDir returns the remote working directory (typically the home dir).
func (c *Client) HomeDir() string {
	wd, err := c.sftpClient.Getwd()
	if err != nil {
		return "/"
	}
	return wd
}

// ListDir returns sorted directory entries: directories first, then files.
func (c *Client) ListDir(path string) ([]FileInfo, error) {
	entries, err := c.sftpClient.ReadDir(path)
	if err != nil {
		return nil, err
	}

	var dirs, files []FileInfo
	for _, e := range entries {
		fi := FileInfo{
			Name:    e.Name(),
			IsDir:   e.IsDir(),
			Size:    e.Size(),
			Mode:    e.Mode(),
			ModTime: e.ModTime(),
		}
		if e.IsDir() {
			dirs = append(dirs, fi)
		} else {
			files = append(files, fi)
		}
	}
	sort.Slice(dirs, func(i, j int) bool { return dirs[i].Name < dirs[j].Name })
	sort.Slice(files, func(i, j int) bool { return files[i].Name < files[j].Name })
	return append(dirs, files...), nil
}

// Upload copies a local file to remotePath on the server.
func (c *Client) Upload(localPath, remotePath string, progress func(int64)) error {
	src, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := c.sftpClient.Create(remotePath)
	if err != nil {
		return err
	}
	defer dst.Close()

	_, err = io.Copy(dst, progressWrap(src, progress))
	return err
}

// Download copies a remote file to localPath.
func (c *Client) Download(remotePath, localPath string, progress func(int64)) error {
	src, err := c.sftpClient.Open(remotePath)
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := os.Create(localPath)
	if err != nil {
		return err
	}
	defer dst.Close()

	_, err = io.Copy(dst, progressWrap(src, progress))
	return err
}

// MkdirRemote creates a directory (and all parents) on the remote.
func (c *Client) MkdirRemote(path string) error {
	return c.sftpClient.MkdirAll(path)
}

// RemoveRemote removes a remote file or empty directory.
func (c *Client) RemoveRemote(path string) error {
	return c.sftpClient.Remove(path)
}

// RemoveRemoteAll recursively removes a remote path (file or directory).
func (c *Client) RemoveRemoteAll(path string) error {
	entries, err := c.sftpClient.ReadDir(path)
	if err != nil {
		// Not a directory or doesn't exist; attempt direct removal.
		return c.sftpClient.Remove(path)
	}
	for _, e := range entries {
		child := path + "/" + e.Name()
		if e.IsDir() {
			if err := c.RemoveRemoteAll(child); err != nil {
				return err
			}
		} else {
			if err := c.sftpClient.Remove(child); err != nil {
				return err
			}
		}
	}
	return c.sftpClient.RemoveDirectory(path)
}

// CreateRemoteFile creates an empty file at the given remote path.
func (c *Client) CreateRemoteFile(path string) error {
	f, err := c.sftpClient.Create(path)
	if err != nil {
		return err
	}
	return f.Close()
}

// UploadDir recursively uploads localDir to remoteDir.
// onFile is called with the relative path before each file upload.
// ctx cancellation stops the walk after the current file finishes.
func (c *Client) UploadDir(ctx context.Context, localDir, remoteDir string, onFile func(rel string)) error {
	return filepath.Walk(localDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if ctx != nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}

		rel, err := filepath.Rel(localDir, path)
		if err != nil {
			return err
		}
		remotePath := remoteDir + "/" + filepath.ToSlash(rel)

		if info.IsDir() {
			return c.sftpClient.MkdirAll(remotePath)
		}
		if onFile != nil {
			onFile(rel)
		}
		return c.Upload(path, remotePath, nil)
	})
}

// DownloadDir recursively downloads remoteDir to localDir.
// onFile is called with the file name before each download.
// ctx cancellation stops the recursion after the current file finishes.
func (c *Client) DownloadDir(ctx context.Context, remoteDir, localDir string, onFile func(name string)) error {
	if ctx != nil {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}
	entries, err := c.sftpClient.ReadDir(remoteDir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(localDir, 0o755); err != nil {
		return err
	}
	for _, e := range entries {
		if ctx != nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
		}
		rPath := remoteDir + "/" + e.Name()
		lPath := filepath.Join(localDir, e.Name())
		if e.IsDir() {
			if err := c.DownloadDir(ctx, rPath, lPath, onFile); err != nil {
				return err
			}
		} else {
			if onFile != nil {
				onFile(e.Name())
			}
			if err := c.Download(rPath, lPath, nil); err != nil {
				return err
			}
		}
	}
	return nil
}

// ReadRemoteFile reads up to limit bytes from the remote file and returns them.
func (c *Client) ReadRemoteFile(path string, limit int64) ([]byte, error) {
	f, err := c.sftpClient.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, limit))
}

// --- known_hosts ------------------------------------------------------------

// loadTolerantKnownHosts reads path line by line, silently skipping any
// malformed entries, and returns a HostKeyCallback backed by the valid subset.
// Returns (nil, nil) if the file does not exist.
func loadTolerantKnownHosts(path string) (gossh.HostKeyCallback, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// Write only the valid lines to a temp file that knownhosts.New can read.
	tmp, err := os.CreateTemp("", "lazyssh-kh-*")
	if err != nil {
		return nil, err
	}
	tmpPath := tmp.Name()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Blank lines and comments are always valid — pass through as-is.
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			fmt.Fprintln(tmp, line)
			continue
		}

		// Validate the entry by attempting to parse it.
		// gossh.ParseKnownHosts returns an error for any malformed line.
		if _, _, _, _, _, pErr := gossh.ParseKnownHosts([]byte(line)); pErr == nil {
			fmt.Fprintln(tmp, line)
		}
		// Silently skip malformed lines (e.g. "192.168.1.160: Connection closed").
	}
	tmp.Close() // must close before knownhosts.New reads it

	// Temp file is only needed while knownhosts.New parses it — remove after.
	defer os.Remove(tmpPath)

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	cb, err := knownhosts.New(tmpPath)
	if err != nil {
		return nil, err
	}
	return cb, nil
}

// appendKnownHost appends a correctly-formatted host key line to the
// known_hosts file. hostname is the raw dial address ("host:port");
// knownhosts.Line normalises it to "host" (port 22) or "[host]:port".
func appendKnownHost(path, hostname string, key gossh.PublicKey) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = fmt.Fprintln(f, knownhosts.Line([]string{hostname}, key))
	return err
}

// --- auth -------------------------------------------------------------------

// buildAuth constructs SSH auth methods for the host.
// Returns (methods, agentConn, error). agentConn is non-nil if an SSH agent
// socket was opened and must be closed when the session ends.
func buildAuth(host *model.Host, password string) ([]gossh.AuthMethod, io.Closer, error) {
	var methods []gossh.AuthMethod
	var agentConn io.Closer

	// 1. SSH agent — only add if the agent actually has identities loaded.
	//    Adding an empty agent wastes a MaxAuthTries slot on many servers.
	if sock := os.Getenv("SSH_AUTH_SOCK"); sock != "" {
		if conn, err := net.Dial("unix", sock); err == nil {
			agentClient := agent.NewClient(conn)
			if signers, err := agentClient.Signers(); err == nil && len(signers) > 0 {
				methods = append(methods, gossh.PublicKeys(signers...))
				agentConn = conn
			} else {
				conn.Close()
			}
		}
	}

	// 2. Explicit key path (AuthType="key", KeyPath set).
	if host.AuthType == model.AuthTypeKey && host.KeyPath != "" {
		keyPath := expandHome(host.KeyPath)
		keyData, err := os.ReadFile(keyPath)
		if err != nil {
			if agentConn != nil {
				agentConn.Close()
			}
			return nil, nil, fmt.Errorf("read key %s: %w", keyPath, err)
		}
		signer, err := gossh.ParsePrivateKey(keyData)
		if err != nil {
			if agentConn != nil {
				agentConn.Close()
			}
			return nil, nil, fmt.Errorf("parse key: %w", err)
		}
		methods = append(methods, gossh.PublicKeys(signer))
	}

	// 3. Default key files (when no explicit path given).
	if host.KeyPath == "" {
		home := os.Getenv("HOME")
		for _, name := range []string{"id_ed25519", "id_rsa", "id_ecdsa", "id_dsa"} {
			keyData, err := os.ReadFile(filepath.Join(home, ".ssh", name))
			if err != nil {
				continue
			}
			signer, err := gossh.ParsePrivateKey(keyData)
			if err != nil {
				continue
			}
			methods = append(methods, gossh.PublicKeys(signer))
		}
	}

	// 4. Password + keyboard-interactive (many servers use the latter even for
	//    what they call "password" auth).
	if password != "" {
		methods = append(methods, gossh.Password(password))
		pwd := password
		methods = append(methods, gossh.KeyboardInteractive(
			func(_, _ string, questions []string, _ []bool) ([]string, error) {
				answers := make([]string, len(questions))
				for i := range answers {
					answers[i] = pwd
				}
				return answers, nil
			},
		))
	}

	if len(methods) == 0 {
		if agentConn != nil {
			agentConn.Close()
		}
		return nil, nil, fmt.Errorf("no auth method available for %s (add a key or password)", host.UserHost())
	}
	return methods, agentConn, nil
}

func expandHome(path string) string {
	if len(path) > 1 && path[0] == '~' {
		return filepath.Join(os.Getenv("HOME"), path[1:])
	}
	return path
}

// --- progress ---------------------------------------------------------------

func progressWrap(r io.Reader, progress func(int64)) io.Reader {
	if progress == nil {
		return r
	}
	return &progressReader{r: r, fn: progress}
}

type progressReader struct {
	r     io.Reader
	fn    func(int64)
	total int64
}

func (pr *progressReader) Read(b []byte) (int, error) {
	n, err := pr.r.Read(b)
	if n > 0 {
		pr.total += int64(n)
		pr.fn(pr.total)
	}
	return n, err
}

// IsAuthError reports whether err is an SSH authentication failure.
// Used by the UI to decide whether to prompt the user for a password.
func IsAuthError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "unable to authenticate") ||
		strings.Contains(msg, "no supported methods remain") ||
		strings.Contains(msg, "no auth method")
}
