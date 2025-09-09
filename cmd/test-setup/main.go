//go:build testsetup
// +build testsetup

package main

import (
	"fmt"
	"log"

	"github.com/belphemur/CBZOptimizer/v2/pkg/converter/webp"
)

func main() {
	fmt.Println("Setting up WebP encoder for tests...")
	if err := webp.PrepareEncoder(); err != nil {
		log.Fatalf("Failed to prepare WebP encoder: %v", err)
	}
	fmt.Println("WebP encoder setup complete.")
}
