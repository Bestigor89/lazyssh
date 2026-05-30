package model

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
)

// Auth type constants.
const (
	AuthTypeKey      = "key"
	AuthTypePassword = "password"
)

// Host represents a saved SSH connection.
type Host struct {
	ID       string   `yaml:"id"`
	Name     string   `yaml:"name"`
	Hostname string   `yaml:"hostname"`
	Port     int      `yaml:"port"`
	User     string   `yaml:"user"`
	AuthType string   `yaml:"auth_type"`
	KeyPath  string   `yaml:"key_path,omitempty"`
	Password string   `yaml:"password,omitempty"` // may be "enc:<base64>" if encrypted
	Folder   string   `yaml:"folder,omitempty"`   // e.g. "prod/web"
	Tags     []string `yaml:"tags,omitempty"`
}

// Store is the top-level configuration written to disk.
type Store struct {
	Version string  `yaml:"version"`
	Hosts   []*Host `yaml:"hosts"`
}

// NewStore returns an empty store ready for use.
func NewStore() *Store {
	return &Store{Version: "1", Hosts: []*Host{}}
}

// NewID generates a random 8-byte hex identifier.
func NewID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// EffectivePort returns Port if non-zero, otherwise 22.
func (h *Host) EffectivePort() int {
	if h.Port > 0 {
		return h.Port
	}
	return 22
}

// Addr returns "hostname:port".
func (h *Host) Addr() string {
	return fmt.Sprintf("%s:%d", h.Hostname, h.EffectivePort())
}

// UserHost returns "user@hostname".
func (h *Host) UserHost() string {
	return fmt.Sprintf("%s@%s", h.User, h.Hostname)
}

// FolderSegments splits the folder path into individual directory segments.
func (h *Host) FolderSegments() []string {
	cleaned := strings.Trim(h.Folder, "/")
	if cleaned == "" {
		return nil
	}
	return strings.Split(cleaned, "/")
}

// Clone returns a deep copy of the host.
func (h *Host) Clone() *Host {
	c := *h
	if h.Tags != nil {
		c.Tags = make([]string, len(h.Tags))
		copy(c.Tags, h.Tags)
	}
	return &c
}
