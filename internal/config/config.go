package config

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/igorivitskyy/lazyssh/internal/model"
)

// Path returns the path to the hosts configuration file.
func Path() string {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		dir = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(dir, "lazyssh", "hosts.yaml")
}

// Load reads the hosts file and returns a Store.
// Returns an empty Store if the file does not exist.
func Load() (*model.Store, error) {
	f, err := os.Open(Path())
	if os.IsNotExist(err) {
		return model.NewStore(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("open config: %w", err)
	}
	defer f.Close()

	var store model.Store
	if err := yaml.NewDecoder(f).Decode(&store); err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}
	if store.Hosts == nil {
		store.Hosts = []*model.Host{}
	}
	return &store, nil
}

// Save writes the store to the hosts file with mode 0600.
func Save(store *model.Store) error {
	p := Path()
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}

	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("create config: %w", err)
	}
	defer f.Close()

	enc := yaml.NewEncoder(f)
	enc.SetIndent(2)
	if err := enc.Encode(store); err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	return nil
}

// Export copies the hosts file to destPath.
func Export(destPath string) error {
	src, err := os.Open(Path())
	if err != nil {
		return fmt.Errorf("open source config: %w", err)
	}
	defer src.Close()

	dst, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("create export file: %w", err)
	}
	defer dst.Close()

	_, err = io.Copy(dst, src)
	return err
}

// Import reads hosts from srcPath and merges them into store.
// Hosts with matching IDs are replaced; new hosts are appended.
// Calls Save after merging.
func Import(store *model.Store, srcPath string) error {
	f, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("open import file: %w", err)
	}
	defer f.Close()

	var imported model.Store
	if err := yaml.NewDecoder(f).Decode(&imported); err != nil {
		return fmt.Errorf("decode import: %w", err)
	}

	existing := make(map[string]int)
	for i, h := range store.Hosts {
		existing[h.ID] = i
	}

	for _, h := range imported.Hosts {
		if idx, ok := existing[h.ID]; ok {
			store.Hosts[idx] = h
		} else {
			store.Hosts = append(store.Hosts, h)
		}
	}

	return Save(store)
}
