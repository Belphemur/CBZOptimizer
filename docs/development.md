# Development Guide

## Build

```bash
go build -o cbzconverter ./cmd/cbzoptimizer
```

## Encoder setup (required for WebP conversion)

```bash
go build -tags encoder_setup -o encoder-setup ./cmd/encoder-setup
./encoder-setup
```

## Test

```bash
go test ./...
```

## Lint

```bash
golangci-lint run
```

## Important behavior

- Input supports CBZ and CBR.
- Output is always CBZ.
- `--override` replaces source CBZ files and removes source CBR files after successful conversion.
- Watch mode performs recursive directory monitoring.
