package manga

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestChapter_SetConverted(t *testing.T) {
	chapter := &Chapter{}

	if chapter.IsConverted {
		t.Error("New chapter should not be converted")
	}
	if !chapter.ConvertedTime.IsZero() {
		t.Error("New chapter should have zero ConvertedTime")
	}

	before := time.Now()
	chapter.SetConverted()
	after := time.Now()

	if !chapter.IsConverted {
		t.Error("Chapter should be converted after SetConverted()")
	}
	if chapter.ConvertedTime.Before(before) || chapter.ConvertedTime.After(after) {
		t.Error("ConvertedTime should be between before and after")
	}
}

func TestChapter_Cleanup(t *testing.T) {
	// Create a temp dir with some files
	tmpDir := t.TempDir()
	chapterDir := filepath.Join(tmpDir, "chapter-cleanup-test")
	if err := os.MkdirAll(filepath.Join(chapterDir, "input"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(chapterDir, "input", "page.jpg"), []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	chapter := &Chapter{TempDir: chapterDir}

	err := chapter.Cleanup()
	if err != nil {
		t.Fatalf("Cleanup failed: %v", err)
	}

	if chapter.TempDir != "" {
		t.Error("TempDir should be empty after cleanup")
	}

	if _, err := os.Stat(chapterDir); !os.IsNotExist(err) {
		t.Error("Directory should not exist after cleanup")
	}
}

func TestChapter_Cleanup_EmptyTempDir(t *testing.T) {
	chapter := &Chapter{TempDir: ""}

	err := chapter.Cleanup()
	if err != nil {
		t.Errorf("Cleanup with empty TempDir should not error, got: %v", err)
	}
}

func TestChapter_Cleanup_NonexistentDir(t *testing.T) {
	chapter := &Chapter{TempDir: "/nonexistent/path/that/does/not/exist"}

	// os.RemoveAll on a nonexistent path returns nil
	err := chapter.Cleanup()
	if err != nil {
		t.Errorf("Cleanup of nonexistent dir should not error, got: %v", err)
	}
}

func TestPageFile_Struct(t *testing.T) {
	page := &PageFile{
		Index:          5,
		Extension:      ".webp",
		FilePath:       "/tmp/test/0005.webp",
		IsSplitted:     true,
		SplitPartIndex: 2,
	}

	if page.Index != 5 {
		t.Errorf("Expected Index 5, got %d", page.Index)
	}
	if page.Extension != ".webp" {
		t.Errorf("Expected Extension .webp, got %s", page.Extension)
	}
	if page.FilePath != "/tmp/test/0005.webp" {
		t.Errorf("Expected FilePath /tmp/test/0005.webp, got %s", page.FilePath)
	}
	if !page.IsSplitted {
		t.Error("Expected IsSplitted true")
	}
	if page.SplitPartIndex != 2 {
		t.Errorf("Expected SplitPartIndex 2, got %d", page.SplitPartIndex)
	}
}
