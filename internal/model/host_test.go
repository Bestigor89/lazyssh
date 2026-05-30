package model

import (
	"reflect"
	"testing"
)

func TestEffectivePort(t *testing.T) {
	tests := []struct {
		port int
		want int
	}{
		{0, 22},
		{-1, 22},
		{22, 22},
		{2222, 2222},
		{46202, 46202},
	}
	for _, tt := range tests {
		h := &Host{Port: tt.port}
		if got := h.EffectivePort(); got != tt.want {
			t.Errorf("Port=%d: want %d, got %d", tt.port, tt.want, got)
		}
	}
}

func TestFolderSegments(t *testing.T) {
	tests := []struct {
		folder string
		want   []string
	}{
		{"", nil},
		{"blazing", []string{"blazing"}},
		{"blazing/chat/live", []string{"blazing", "chat", "live"}},
		{"/blazing/chat/", []string{"blazing", "chat"}},
	}
	for _, tt := range tests {
		h := &Host{Folder: tt.folder}
		got := h.FolderSegments()
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("Folder=%q: want %v, got %v", tt.folder, tt.want, got)
		}
	}
}

func TestUserHost(t *testing.T) {
	h := &Host{User: "root", Hostname: "192.168.1.1"}
	if got := h.UserHost(); got != "root@192.168.1.1" {
		t.Errorf("UserHost: got %q", got)
	}
}

func TestNewIDUniqueness(t *testing.T) {
	ids := make(map[string]bool, 100)
	for i := 0; i < 100; i++ {
		id := NewID()
		if len(id) != 16 {
			t.Fatalf("expected 16-char hex ID, got %q (len=%d)", id, len(id))
		}
		if ids[id] {
			t.Fatalf("duplicate ID generated: %q", id)
		}
		ids[id] = true
	}
}
