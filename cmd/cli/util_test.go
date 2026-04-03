package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveSSHKey_Empty(t *testing.T) {
	if got := resolveSSHKey(""); got != "" {
		t.Errorf("resolveSSHKey(\"\"): got %q, want empty", got)
	}
}

func TestResolveSSHKey_LiteralKey(t *testing.T) {
	key := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAA test@host"
	if got := resolveSSHKey(key); got != key {
		t.Errorf("resolveSSHKey: got %q, want %q", got, key)
	}
}

func TestResolveSSHKey_ECDSAKey(t *testing.T) {
	key := "ecdsa-sha2-nistp256 AAAA test@host"
	if got := resolveSSHKey(key); got != key {
		t.Errorf("resolveSSHKey: got %q, want %q", got, key)
	}
}

func TestResolveSSHKey_FromFile(t *testing.T) {
	dir := t.TempDir()
	keyFile := filepath.Join(dir, "id_ed25519.pub")
	keyContent := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAA test@host"
	_ = os.WriteFile(keyFile, []byte(keyContent+"\n"), 0644)

	got := resolveSSHKey(keyFile)
	if got != keyContent {
		t.Errorf("resolveSSHKey from file: got %q, want %q", got, keyContent)
	}
}

func TestResolveSSHKey_FileTilde(t *testing.T) {
	// Create a temp file in a known location
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home dir")
	}

	tmpDir := filepath.Join(home, ".v-test-tmp")
	_ = os.MkdirAll(tmpDir, 0750)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	keyFile := filepath.Join(tmpDir, "test.pub")
	keyContent := "ssh-rsa AAAA test@host"
	_ = os.WriteFile(keyFile, []byte(keyContent), 0644)

	got := resolveSSHKey("~/.v-test-tmp/test.pub")
	if got != keyContent {
		t.Errorf("resolveSSHKey with ~: got %q, want %q", got, keyContent)
	}
}

func TestResolveSSHKey_FileNotFound(t *testing.T) {
	// When file doesn't exist, treat input as literal
	got := resolveSSHKey("/nonexistent/path/key.pub")
	if got != "/nonexistent/path/key.pub" {
		t.Errorf("resolveSSHKey nonexistent: got %q, want literal path", got)
	}
}

func TestSplitLast(t *testing.T) {
	tests := []struct {
		s, sep, want string
	}{
		{"a/b/c", "/", "c"},
		{"https://example.com/path/to/file.img", "/", "file.img"},
		{"nosep", "/", "nosep"},
		{"a/", "/", ""},
		{"/single", "/", "single"},
	}
	for _, tt := range tests {
		got := splitLast(tt.s, tt.sep)
		if got != tt.want {
			t.Errorf("splitLast(%q, %q) = %q, want %q", tt.s, tt.sep, got, tt.want)
		}
	}
}
