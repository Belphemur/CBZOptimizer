package utils

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/belphemur/CBZOptimizer/v2/internal/cbz"
	"github.com/belphemur/CBZOptimizer/v2/internal/manga"
	"github.com/belphemur/CBZOptimizer/v2/internal/utils/errs"
	"github.com/belphemur/CBZOptimizer/v2/pkg/converter/constant"
)

// MockConverter for testing
type MockConverter struct {
	shouldFail bool
}

func (m *MockConverter) ConvertChapter(ctx context.Context, chapter *manga.Chapter, quality uint8, split bool, progress func(message string, current uint32, total uint32)) (*manga.Chapter, error) {
	if m.shouldFail {
		return nil, &MockError{message: "mock conversion error"}
	}

	// Check if context is already cancelled
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Simulate some work that can be interrupted by context cancellation
	for i := 0; i < len(chapter.Pages); i++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			// Simulate processing time
			time.Sleep(100 * time.Microsecond)
			if progress != nil {
				progress(fmt.Sprintf("Converting page %d/%d", i+1, len(chapter.Pages)), uint32(i+1), uint32(len(chapter.Pages)))
			}
		}
	}

	// Create a copy of the chapter to simulate conversion
	converted := &manga.Chapter{
		FilePath:      chapter.FilePath,
		Pages:         chapter.Pages,
		ComicInfoXml:  chapter.ComicInfoXml,
		IsConverted:   true,
		ConvertedTime: time.Now(),
	}
	return converted, nil
}

func (m *MockConverter) Format() constant.ConversionFormat {
	return constant.WebP
}

func (m *MockConverter) PrepareConverter() error {
	if m.shouldFail {
		return &MockError{message: "mock prepare error"}
	}
	return nil
}

type MockError struct {
	message string
}

func (e *MockError) Error() string {
	return e.message
}

func TestOptimize(t *testing.T) {
	// Create temporary directory for tests
	tempDir, err := os.MkdirTemp("", "test_optimize")
	if err != nil {
		t.Fatal(err)
	}
	defer errs.CaptureGeneric(&err, os.RemoveAll, tempDir, "failed to remove temporary directory")

	// Copy test files
	testdataDir := "../../testdata"
	if _, err := os.Stat(testdataDir); os.IsNotExist(err) {
		t.Skip("testdata directory not found, skipping tests")
	}

	// Copy sample files
	var cbzFile, cbrFile string
	err = filepath.Walk(testdataDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			fileName := strings.ToLower(info.Name())
			if strings.HasSuffix(fileName, ".cbz") && !strings.Contains(fileName, "converted") {
				destPath := filepath.Join(tempDir, "test.cbz")
				data, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				err = os.WriteFile(destPath, data, info.Mode())
				if err != nil {
					return err
				}
				cbzFile = destPath
			} else if strings.HasSuffix(fileName, ".cbr") {
				destPath := filepath.Join(tempDir, "test.cbr")
				data, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				err = os.WriteFile(destPath, data, info.Mode())
				if err != nil {
					return err
				}
				cbrFile = destPath
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	if cbzFile == "" {
		t.Skip("No CBZ test file found")
	}

	// Create a CBR file by copying the CBZ file if no CBR exists
	if cbrFile == "" {
		cbrFile = filepath.Join(tempDir, "test.cbr")
		data, err := os.ReadFile(cbzFile)
		if err != nil {
			t.Fatal(err)
		}
		err = os.WriteFile(cbrFile, data, 0644)
		if err != nil {
			t.Fatal(err)
		}
	}

	tests := []struct {
		name           string
		inputFile      string
		override       bool
		expectedOutput string
		shouldDelete   bool
		expectError    bool
		mockFail       bool
	}{
		{
			name:           "CBZ file without override",
			inputFile:      cbzFile,
			override:       false,
			expectedOutput: strings.TrimSuffix(cbzFile, ".cbz") + "_converted.cbz",
			shouldDelete:   false,
			expectError:    false,
		},
		{
			name:           "CBZ file with override",
			inputFile:      cbzFile,
			override:       true,
			expectedOutput: cbzFile,
			shouldDelete:   false,
			expectError:    false,
		},
		{
			name:           "CBR file without override",
			inputFile:      cbrFile,
			override:       false,
			expectedOutput: strings.TrimSuffix(cbrFile, ".cbr") + "_converted.cbz",
			shouldDelete:   false,
			expectError:    false,
		},
		{
			name:           "CBR file with override",
			inputFile:      cbrFile,
			override:       true,
			expectedOutput: strings.TrimSuffix(cbrFile, ".cbr") + ".cbz",
			shouldDelete:   true,
			expectError:    false,
		},
		{
			name:        "Converter failure",
			inputFile:   cbzFile,
			override:    false,
			expectError: true,
			mockFail:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a copy of the input file for this test
			testFile := filepath.Join(tempDir, tt.name+"_"+filepath.Base(tt.inputFile))
			data, err := os.ReadFile(tt.inputFile)
			if err != nil {
				t.Fatal(err)
			}
			err = os.WriteFile(testFile, data, 0644)
			if err != nil {
				t.Fatal(err)
			}

			// Setup options
			options := &OptimizeOptions{
				ChapterConverter: &MockConverter{shouldFail: tt.mockFail},
				Path:             testFile,
				Quality:          85,
				Override:         tt.override,
				Split:            false,
				Timeout:          0,
			}

			// Run optimization
			err = Optimize(options)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			// Determine expected output path for this test
			expectedOutput := tt.expectedOutput
			if tt.override && strings.HasSuffix(strings.ToLower(testFile), ".cbr") {
				expectedOutput = strings.TrimSuffix(testFile, filepath.Ext(testFile)) + ".cbz"
			} else if !tt.override {
				if strings.HasSuffix(strings.ToLower(testFile), ".cbz") {
					expectedOutput = strings.TrimSuffix(testFile, ".cbz") + "_converted.cbz"
				} else if strings.HasSuffix(strings.ToLower(testFile), ".cbr") {
					expectedOutput = strings.TrimSuffix(testFile, ".cbr") + "_converted.cbz"
				}
			} else {
				expectedOutput = testFile
			}

			// Verify output file exists
			if _, err := os.Stat(expectedOutput); os.IsNotExist(err) {
				t.Errorf("Expected output file not found: %s", expectedOutput)
			}

			// Verify output is a valid CBZ
			chapter, err := cbz.LoadChapter(expectedOutput)
			if err != nil {
				t.Errorf("Failed to load converted chapter: %v", err)
			}

			if !chapter.IsConverted {
				t.Error("Chapter is not marked as converted")
			}

			// Verify original file deletion for CBR override
			if tt.shouldDelete {
				if _, err := os.Stat(testFile); !os.IsNotExist(err) {
					t.Error("Original CBR file should have been deleted but still exists")
				}
			} else {
				// Verify original file still exists (unless it's the same as output)
				if testFile != expectedOutput {
					if _, err := os.Stat(testFile); os.IsNotExist(err) {
						t.Error("Original file should not have been deleted")
					}
				}
			}

			// Clean up output file
			_ = os.Remove(expectedOutput)
		})
	}
}

func TestOptimize_AlreadyConverted(t *testing.T) {
	// Create temporary directory
	tempDir, err := os.MkdirTemp("", "test_optimize_converted")
	if err != nil {
		t.Fatal(err)
	}
	defer errs.CaptureGeneric(&err, os.RemoveAll, tempDir, "failed to remove temporary directory")

	// Use a converted test file
	testdataDir := "../../testdata"
	if _, err := os.Stat(testdataDir); os.IsNotExist(err) {
		t.Skip("testdata directory not found, skipping tests")
	}

	var convertedFile string
	err = filepath.Walk(testdataDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.Contains(strings.ToLower(info.Name()), "converted") {
			destPath := filepath.Join(tempDir, info.Name())
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			err = os.WriteFile(destPath, data, info.Mode())
			if err != nil {
				return err
			}
			convertedFile = destPath
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	if convertedFile == "" {
		t.Skip("No converted test file found")
	}

	options := &OptimizeOptions{
		ChapterConverter: &MockConverter{},
		Path:             convertedFile,
		Quality:          85,
		Override:         false,
		Split:            false,
		Timeout:          0,
	}

	err = Optimize(options)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Should not create a new file since it's already converted
	expectedOutput := strings.TrimSuffix(convertedFile, ".cbz") + "_converted.cbz"
	if _, err := os.Stat(expectedOutput); !os.IsNotExist(err) {
		t.Error("Should not have created a new converted file for already converted chapter")
	}
}

func TestOptimize_InvalidFile(t *testing.T) {
	options := &OptimizeOptions{
		ChapterConverter: &MockConverter{},
		Path:             "/nonexistent/file.cbz",
		Quality:          85,
		Override:         false,
		Split:            false,
		Timeout:          0,
	}

	err := Optimize(options)
	if err == nil {
		t.Error("Expected error for nonexistent file")
	}
}

func TestOptimize_Timeout(t *testing.T) {
	// Create temporary directory
	tempDir, err := os.MkdirTemp("", "test_optimize_timeout")
	if err != nil {
		t.Fatal(err)
	}
	defer errs.CaptureGeneric(&err, os.RemoveAll, tempDir, "failed to remove temporary directory")

	// Copy test files
	testdataDir := "../../testdata"
	if _, err := os.Stat(testdataDir); os.IsNotExist(err) {
		t.Skip("testdata directory not found, skipping tests")
	}

	var cbzFile string
	err = filepath.Walk(testdataDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(strings.ToLower(info.Name()), ".cbz") && !strings.Contains(info.Name(), "converted") {
			destPath := filepath.Join(tempDir, "test.cbz")
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			err = os.WriteFile(destPath, data, info.Mode())
			if err != nil {
				return err
			}
			cbzFile = destPath
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	if cbzFile == "" {
		t.Skip("No CBZ test file found")
	}

	// Test with short timeout (500 microseconds) to force timeout during conversion
	options := &OptimizeOptions{
		ChapterConverter: &MockConverter{},
		Path:             cbzFile,
		Quality:          85,
		Override:         false,
		Split:            false,
		Timeout:          500 * time.Microsecond, // 500 microseconds - should timeout during page processing
	}

	err = Optimize(options)
	if err == nil {
		t.Error("Expected timeout error but got none")
	}

	// Check that the error contains timeout information
	if !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Errorf("Expected timeout error message, got: %v", err)
	}
}
