package cbz

import (
	"strings"
	"testing"
)

func TestLoadChapter(t *testing.T) {
	type testCase struct {
		name               string
		filePath           string
		expectedPages      int
		expectedSeries     string
		expectedConversion bool
	}

	testCases := []testCase{
		{
			name:               "Original Chapter",
			filePath:           "../testdata/Chapter 1.cbz",
			expectedPages:      16,
			expectedSeries:     "<Series>Boundless Necromancer</Series>",
			expectedConversion: false,
		},
		{
			name:               "Converted Chapter",
			filePath:           "../testdata/Chapter 10_converted.cbz",
			expectedPages:      107,
			expectedSeries:     "<Series>Boundless Necromancer</Series>",
			expectedConversion: true,
		},
		// Add more test cases as needed
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			chapter, err := LoadChapter(tc.filePath)
			if err != nil {
				t.Fatalf("Failed to load chapter: %v", err)
			}

			actualPages := len(chapter.Pages)
			if actualPages != tc.expectedPages {
				t.Errorf("Expected %d pages, but got %d", tc.expectedPages, actualPages)
			}

			if !strings.Contains(chapter.ComicInfoXml, tc.expectedSeries) {
				t.Errorf("ComicInfoXml does not contain the expected series: %s", tc.expectedSeries)
			}

			if chapter.IsConverted != tc.expectedConversion {
				t.Errorf("Expected chapter to be converted: %t, but got %t", tc.expectedConversion, chapter.IsConverted)
			}
		})
	}
}
