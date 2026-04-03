package cli

import (
	"os"
	"strings"
)

func readFileArg(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// resolveSSHKey returns the SSH key content. If the input looks like a file
// path (e.g. ends with .pub or starts with / or ~), it reads the file.
// Otherwise it's treated as a literal key string.
func resolveSSHKey(input string) string {
	if input == "" {
		return ""
	}
	if strings.HasPrefix(input, "ssh-") || strings.HasPrefix(input, "ecdsa-") {
		return input
	}
	// Expand ~ to home dir
	if strings.HasPrefix(input, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			input = home + input[1:]
		}
	}
	data, err := os.ReadFile(input)
	if err != nil {
		return input // treat as literal key if file not found
	}
	return strings.TrimSpace(string(data))
}
