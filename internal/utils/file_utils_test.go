package utils

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsValidFolder(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(t *testing.T) string
		expected bool
	}{
		{
			name: "valid directory",
			setup: func(t *testing.T) string {
				return t.TempDir()
			},
			expected: true,
		},
		{
			name: "file not directory",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				path := filepath.Join(dir, "file.txt")
				if err := os.WriteFile(path, []byte("content"), 0644); err != nil {
					t.Fatal(err)
				}
				return path
			},
			expected: false,
		},
		{
			name: "nonexistent path",
			setup: func(t *testing.T) string {
				return "/nonexistent/path/that/does/not/exist"
			},
			expected: false,
		},
		{
			name: "empty string",
			setup: func(t *testing.T) string {
				return ""
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.setup(t)
			result := IsValidFolder(path)
			assert.Equal(t, tt.expected, result)
		})
	}
}
