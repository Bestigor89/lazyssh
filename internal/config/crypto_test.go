package config

import (
	"strings"
	"testing"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	master := "super-secret"
	plain := "my-ssh-password"

	enc, err := Encrypt(master, plain)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if !strings.HasPrefix(enc, "enc:") {
		t.Fatalf("expected enc: prefix, got %q", enc)
	}

	got, err := Decrypt(master, enc)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if got != plain {
		t.Fatalf("round-trip mismatch: want %q, got %q", plain, got)
	}
}

func TestDecryptWrongPassword(t *testing.T) {
	enc, err := Encrypt("correct-password", "secret")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	_, err = Decrypt("wrong-password", enc)
	if err == nil {
		t.Fatal("expected error with wrong master password, got nil")
	}
}

func TestDecryptPlaintext(t *testing.T) {
	// Non-encrypted values must be returned unchanged.
	got, err := Decrypt("any-master", "plaintext-password")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "plaintext-password" {
		t.Fatalf("expected passthrough, got %q", got)
	}
}

func TestIsEncrypted(t *testing.T) {
	enc, _ := Encrypt("pwd", "val")
	if !IsEncrypted(enc) {
		t.Error("expected IsEncrypted=true for an encrypted value")
	}
	if IsEncrypted("plaintext") {
		t.Error("expected IsEncrypted=false for a plain value")
	}
}
