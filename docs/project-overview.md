# Project Overview

CBZOptimizer is a Go CLI that optimizes comic archives (`.cbz` and `.cbr`) by converting page images to modern formats (currently WebP).

## High-level flow

1. Load chapters from archive files.
2. Decode pages and optionally split oversized pages.
3. Convert images with the selected converter.
4. Write optimized output as CBZ.

## Main components

- `cmd/cbzoptimizer`: CLI commands and flag wiring (`optimize`, `watch`).
- `internal/cbz`: archive loading and writing.
- `internal/manga`: chapter and page domain models.
- `internal/utils`: orchestration utilities (`optimize` flow and file helpers).
- `pkg/converter`: converter abstraction and format implementations.

## Watch mode

`watch` monitors a directory tree for archive file changes and runs optimization automatically.

## Key runtime requirements

- Go 1.25+
- WebP encoder setup via `cmd/encoder-setup` for WebP conversion tests and runtime support.
