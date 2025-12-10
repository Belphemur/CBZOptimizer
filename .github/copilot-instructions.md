# CBZOptimizer - GitHub Copilot Instructions

## Project Overview

CBZOptimizer is a Go-based command-line tool designed to optimize CBZ (Comic Book Zip) and CBR (Comic Book RAR) files by converting images to modern formats (primarily WebP) with configurable quality settings. The tool reduces the size of comic book archives while maintaining acceptable image quality.

**Key Features:**
- Convert CBZ/CBR files to optimized CBZ format
- WebP image encoding with quality control
- Parallel chapter processing
- File watching for automatic optimization
- Optional page splitting for large images
- Timeout handling for problematic files

## Technology Stack

- **Language:** Go 1.25+
- **CLI Framework:** Cobra + Viper
- **Logging:** zerolog (structured logging)
- **Image Processing:** go-webpbin/v2 for WebP encoding
- **Archive Handling:** mholt/archives for CBZ/CBR processing
- **Testing:** testify + gotestsum

## Project Structure

```
.
├── cmd/
│   ├── cbzoptimizer/         # Main CLI application
│   │   ├── commands/         # Cobra commands (optimize, watch)
│   │   └── main.go          # Entry point
│   └── encoder-setup/        # WebP encoder setup utility
│       └── main.go          # Encoder initialization (build tag: encoder_setup)
├── internal/
│   ├── cbz/                 # CBZ/CBR file operations
│   │   ├── cbz_loader.go   # Load and parse comic archives
│   │   └── cbz_creator.go  # Create optimized archives
│   ├── manga/              # Domain models
│   │   ├── chapter.go      # Chapter representation
│   │   ├── page.go         # Page image handling
│   │   └── page_container.go # Page collection management
│   └── utils/              # Utility functions
│       ├── optimize.go     # Core optimization logic
│       └── errs/           # Error handling utilities
└── pkg/
    └── converter/          # Image conversion abstractions
        ├── converter.go    # Converter interface
        ├── webp/          # WebP implementation
        │   ├── webp_converter.go    # WebP conversion logic
        │   └── webp_provider.go     # WebP encoder provider
        ├── errors/        # Conversion error types
        └── constant/      # Shared constants
```

## Building and Testing

### Prerequisites

Before building or testing, the WebP encoder must be set up:

```bash
# Build the encoder-setup utility
go build -tags encoder_setup -o encoder-setup ./cmd/encoder-setup

# Run encoder setup (downloads and configures libwebp 1.6.0)
./encoder-setup
```

This step is **required** before running tests or building the main application.

### Build Commands

```bash
# Build the main application
go build -o cbzoptimizer ./cmd/cbzoptimizer

# Build with version information
go build -ldflags "-s -w -X main.version=1.0.0 -X main.commit=abc123 -X main.date=2024-01-01" ./cmd/cbzoptimizer
```

### Testing

```bash
# Install test runner
go install gotest.tools/gotestsum@latest

# Run all tests with coverage
gotestsum --format testname -- -race -coverprofile=coverage.txt -covermode=atomic ./...

# Run specific package tests
go test -v ./internal/cbz/...
go test -v ./pkg/converter/...

# Run integration tests
go test -v ./internal/utils/...
```

### Linting

```bash
# Install golangci-lint if not available
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Run linter
golangci-lint run
```

## Code Conventions

### Go Style

- **Follow standard Go conventions:** Use `gofmt` and `goimports`
- **Package naming:** Short, lowercase, single-word names
- **Error handling:** Always check errors explicitly; use structured error wrapping with `fmt.Errorf("context: %w", err)`
- **Context usage:** Pass `context.Context` as first parameter for operations that may be cancelled

### Logging

Use **zerolog** for all logging:

```go
import "github.com/rs/zerolog/log"

// Info level with structured fields
log.Info().Str("file", path).Int("pages", count).Msg("Processing file")

// Debug level for detailed diagnostics
log.Debug().Str("file", path).Uint8("quality", quality).Msg("Optimization parameters")

// Error level with error wrapping
log.Error().Str("file", path).Err(err).Msg("Failed to load chapter")
```

**Log Levels (in order of verbosity):**
- `panic` - System panic conditions
- `fatal` - Fatal errors requiring exit
- `error` - Error conditions
- `warn` - Warning conditions
- `info` - General information (default)
- `debug` - Debug-level messages
- `trace` - Trace-level messages

### Error Handling

- Use the custom `errs` package for deferred error handling:
  ```go
  import "github.com/belphemur/CBZOptimizer/v2/internal/utils/errs"
  
  func processFile() (err error) {
      defer errs.Wrap(&err, "failed to process file")
      // ... implementation
  }
  ```

- Define custom error types in `pkg/converter/errors/` for specific error conditions
- Always provide context when wrapping errors

### Testing

- Use **testify** for assertions:
  ```go
  import "github.com/stretchr/testify/assert"
  
  func TestSomething(t *testing.T) {
      result, err := DoSomething()
      assert.NoError(t, err)
      assert.Equal(t, expected, result)
  }
  ```

- Use table-driven tests for multiple scenarios:
  ```go
  testCases := []struct {
      name        string
      input       string
      expected    string
      expectError bool
  }{
      {"case1", "input1", "output1", false},
      {"case2", "input2", "output2", true},
  }
  
  for _, tc := range testCases {
      t.Run(tc.name, func(t *testing.T) {
          // test implementation
      })
  }
  ```

- Integration tests should be in `*_integration_test.go` files
- Use temporary directories for file operations in tests

### Command Structure (Cobra)

- Commands are in `cmd/cbzoptimizer/commands/`
- Each command is in its own file (e.g., `optimize_command.go`, `watch_command.go`)
- Use Cobra's persistent flags for global options
- Use Viper for configuration management

### Dependencies

**Key external packages:**
- `github.com/belphemur/go-webpbin/v2` - WebP encoding (libwebp wrapper)
- `github.com/mholt/archives` - Archive format handling
- `github.com/spf13/cobra` - CLI framework
- `github.com/spf13/viper` - Configuration management
- `github.com/rs/zerolog` - Structured logging
- `github.com/oliamb/cutter` - Image cropping for page splitting
- `golang.org/x/image` - Extended image format support

## Docker Considerations

The Dockerfile uses a multi-stage build and requires:
1. The compiled `CBZOptimizer` binary (from goreleaser)
2. The `encoder-setup` binary (built with `-tags encoder_setup`)
3. The encoder-setup is run during image build to configure WebP encoder

The encoder must be set up in the container before the application runs.

## Common Tasks

### Adding a New Command

1. Create `cmd/cbzoptimizer/commands/newcommand_command.go`
2. Define the command using Cobra:
   ```go
   var newCmd = &cobra.Command{
       Use:   "new",
       Short: "Description",
       RunE: func(cmd *cobra.Command, args []string) error {
           // implementation
       },
   }
   
   func init() {
       rootCmd.AddCommand(newCmd)
   }
   ```
3. Add tests in `newcommand_command_test.go`

### Adding a New Image Format Converter

1. Create a new package under `pkg/converter/` (e.g., `avif/`)
2. Implement the `Converter` interface from `pkg/converter/converter.go`
3. Add tests following existing patterns in `pkg/converter/webp/`
4. Update command flags to support the new format

### Modifying Optimization Logic

The core optimization logic is in `internal/utils/optimize.go`:
- Uses the `OptimizeOptions` struct for parameters
- Handles chapter loading, conversion, and saving
- Implements timeout handling with context
- Provides structured logging at each step

## CI/CD

### GitHub Actions Workflows

1. **test.yml** - Runs on every push/PR
   - Sets up Go environment
   - Runs encoder-setup
   - Executes tests with coverage
   - Uploads results to Codecov

2. **release.yml** - Runs on version tags
   - Uses goreleaser for multi-platform builds
   - Builds Docker images for linux/amd64 and linux/arm64
   - Signs releases with cosign
   - Generates SBOMs with syft

3. **qodana.yml** - Code quality analysis

### Release Process

Releases are automated via goreleaser:
- Tag format: `v*` (e.g., `v2.1.0`)
- Builds for: linux, darwin, windows (amd64, arm64)
- Creates Docker images and pushes to ghcr.io
- Generates checksums and SBOMs

## Performance Considerations

- **Parallelism:** Use `--parallelism` flag to control concurrent chapter processing
- **Memory:** Large images are processed in-memory; consider system RAM when setting parallelism
- **Timeouts:** Use `--timeout` flag to prevent hanging on problematic files
- **WebP Quality:** Balance quality (0-100) vs file size; default is 85

## Security

- No credentials or secrets should be committed
- Archive extraction includes path traversal protection
- File permissions are preserved during operations
- Docker images run as non-root user (`abc`, UID 99)

## Additional Notes

- CBR files are always converted to CBZ format (RAR is read-only)
- The `--override` flag deletes the original file after successful conversion
- Page splitting is useful for double-page spreads or very tall images
- Watch mode uses inotify on Linux for efficient file monitoring
- Bash completion is available via `cbzoptimizer completion bash`

## Getting Help

- Use `--help` flag for command documentation
- Use `--log debug` for detailed diagnostic output
- Check GitHub Issues for known problems
- Review test files for usage examples
