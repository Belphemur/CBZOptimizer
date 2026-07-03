package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/belphemur/CBZOptimizer/v2/internal/cbz"
	"github.com/belphemur/CBZOptimizer/v2/internal/utils/errs"
	"github.com/belphemur/CBZOptimizer/v2/pkg/converter"
	"github.com/belphemur/CBZOptimizer/v2/pkg/converter/constant"
)

func TestOptimizeIntegration(t *testing.T) {
	// Skip integration tests if no libwebp is available or testdata doesn't exist
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Check if testdata directory exists
	testdataDir := "../../testdata"
	if _, err := os.Stat(testdataDir); os.IsNotExist(err) {
		t.Skip("testdata directory not found, skipping integration tests")
	}

	// Create temporary directory for tests
	tempDir, err := os.MkdirTemp("", "test_optimize_integration")
	if err != nil {
		t.Fatal(err)
	}
	defer errs.CaptureGeneric(&err, os.RemoveAll, tempDir, "failed to remove temporary directory")

	// Get the real webp converter
	converterInstance, err := converter.Get(constant.WebP)
	if err != nil {
		t.Skip("WebP converter not available, skipping integration tests")
	}

	// Prepare the converter
	err = converterInstance.PrepareConverter()
	if err != nil {
		t.Skip("Failed to prepare WebP converter, skipping integration tests")
	}

	// Collect all test files (CBZ/CBR, excluding converted ones)
	var testFiles []string
	err = filepath.Walk(testdataDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			fileName := strings.ToLower(info.Name())
			if (strings.HasSuffix(fileName, ".cbz") || strings.HasSuffix(fileName, ".cbr")) && !strings.Contains(fileName, "converted") {
				testFiles = append(testFiles, path)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(testFiles) == 0 {
		t.Skip("No test files found")
	}

	tests := []struct {
		name           string
		inputFile      string
		override       bool
		expectedOutput string
		shouldDelete   bool
		expectError    bool
	}{}

	// Generate test cases for each available test file
	for _, testFile := range testFiles {
		baseName := strings.TrimSuffix(filepath.Base(testFile), filepath.Ext(testFile))
		isCBR := strings.HasSuffix(strings.ToLower(testFile), ".cbr")

		// Test without override
		tests = append(tests, struct {
			name           string
			inputFile      string
			override       bool
			expectedOutput string
			shouldDelete   bool
			expectError    bool
		}{
			name:           fmt.Sprintf("%s file without override", strings.ToUpper(filepath.Ext(testFile)[1:])),
			inputFile:      testFile,
			override:       false,
			expectedOutput: filepath.Join(filepath.Dir(testFile), baseName+"_converted.cbz"),
			shouldDelete:   false,
			expectError:    false,
		})

		// Test with override
		if isCBR {
			tests = append(tests, struct {
				name           string
				inputFile      string
				override       bool
				expectedOutput string
				shouldDelete   bool
				expectError    bool
			}{
				name:           fmt.Sprintf("%s file with override", strings.ToUpper(filepath.Ext(testFile)[1:])),
				inputFile:      testFile,
				override:       true,
				expectedOutput: filepath.Join(filepath.Dir(testFile), baseName+".cbz"),
				shouldDelete:   true,
				expectError:    false,
			})
		}
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

			// Setup options with real converter
			options := &OptimizeOptions{
				ChapterConverter: converterInstance,
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

			// Verify output is a valid CBZ with converted content
			chapter, err := cbz.LoadChapter(expectedOutput)
			if err != nil {
				t.Errorf("Failed to load converted chapter: %v", err)
			}

			if !chapter.IsConverted {
				t.Error("Chapter is not marked as converted")
			}

			// Verify all pages are in WebP format (real conversion indicator)
			for i, page := range chapter.Pages {
				if page.Extension != ".webp" {
					t.Errorf("Page %d is not converted to WebP format (got: %s)", i, page.Extension)
				}
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
			os.Remove(expectedOutput)
		})
	}
}

func TestOptimizeIntegration_AlreadyConverted(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create temporary directory
	tempDir, err := os.MkdirTemp("", "test_optimize_integration_converted")
	if err != nil {
		t.Fatal(err)
	}
	defer errs.CaptureGeneric(&err, os.RemoveAll, tempDir, "failed to remove temporary directory")

	// Use a converted test file
	testdataDir := "../../testdata"
	if _, err := os.Stat(testdataDir); os.IsNotExist(err) {
		t.Skip("testdata directory not found, skipping integration tests")
	}

	// Get the real webp converter
	converterInstance, err := converter.Get(constant.WebP)
	if err != nil {
		t.Skip("WebP converter not available, skipping integration tests")
	}

	// Prepare the converter
	err = converterInstance.PrepareConverter()
	if err != nil {
		t.Skip("Failed to prepare WebP converter, skipping integration tests")
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
		ChapterConverter: converterInstance,
		Path:             convertedFile,
		Quality:          85,
		Override:         false,
		Split:            false,
		Timeout:          30 * time.Second,
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

func TestOptimizeIntegration_InvalidFile(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Get the real webp converter
	converterInstance, err := converter.Get(constant.WebP)
	if err != nil {
		t.Skip("WebP converter not available, skipping integration tests")
	}

	// Prepare the converter
	err = converterInstance.PrepareConverter()
	if err != nil {
		t.Skip("Failed to prepare WebP converter, skipping integration tests")
	}

	options := &OptimizeOptions{
		ChapterConverter: converterInstance,
		Path:             "/nonexistent/file.cbz",
		Quality:          85,
		Override:         false,
		Split:            false,
		Timeout:          30 * time.Second,
	}

	err = Optimize(options)
	if err == nil {
		t.Error("Expected error for nonexistent file")
	}
}

func TestOptimizeIntegration_Timeout(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create temporary directory
	tempDir, err := os.MkdirTemp("", "test_optimize_integration_timeout")
	if err != nil {
		t.Fatal(err)
	}
	defer errs.CaptureGeneric(&err, os.RemoveAll, tempDir, "failed to remove temporary directory")

	// Copy test files
	testdataDir := "../../testdata"
	if _, err := os.Stat(testdataDir); os.IsNotExist(err) {
		t.Skip("testdata directory not found, skipping integration tests")
	}

	// Get the real webp converter
	converterInstance, err := converter.Get(constant.WebP)
	if err != nil {
		t.Skip("WebP converter not available, skipping integration tests")
	}

	// Prepare the converter
	err = converterInstance.PrepareConverter()
	if err != nil {
		t.Skip("Failed to prepare WebP converter, skipping integration tests")
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

	// Test with short timeout to force timeout during conversion
	options := &OptimizeOptions{
		ChapterConverter: converterInstance,
		Path:             cbzFile,
		Quality:          85,
		Override:         false,
		Split:            false,
		Timeout:          10 * time.Millisecond, // Very short timeout to force timeout
	}

	err = Optimize(options)
	if err == nil {
		t.Error("Expected timeout error but got none")
	}

	// Check that the error contains timeout information
	if err != nil && !strings.Contains(err.Error(), "context deadline exceeded") && !strings.Contains(err.Error(), "timeout") {
		t.Errorf("Expected timeout error message, got: %v", err)
	}
}
