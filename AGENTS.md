# Agent Instructions

This repository contains AI-agent oriented project context.

## Read first

- `docs/project-overview.md` for architecture and execution flow.
- `docs/development.md` for build/test/lint commands and runtime prerequisites.

## Repository conventions

- Language: Go
- CLI framework: Cobra + Viper
- Logging: zerolog
- Error wrapping: `fmt.Errorf("context: %w", err)`
- Prefer small, focused changes.
- Commit messages: follow [Conventional Commits](https://www.conventionalcommits.org/) (e.g. `fix(watch): debounce fsnotify events`, `docs: clarify commit convention`).

## Areas to know

- Watch command: `cmd/cbzoptimizer/commands/watch_command.go`
- Optimization orchestration: `internal/utils/optimize.go`
- Converter interface/impl: `pkg/converter/`
