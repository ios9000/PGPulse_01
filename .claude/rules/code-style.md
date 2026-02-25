# Go Code Style

## Formatting
- Use gofmt/goimports (enforced by CI)
- Max line length: 120 characters (soft limit)
- Use table-driven tests

## Naming
- Package names: lowercase, single word (collector, storage, alert)
- Interfaces: verb-based (Collector, Notifier, Store)
- Unexported helpers: descriptive but concise
- Test files: *_test.go in same package

## Error Handling
- Wrap errors with fmt.Errorf("context: %w", err)
- Never silently ignore errors
- Use structured logging (slog) for error reporting
- Return early on errors (guard clauses)

## Dependencies
- Minimize external dependencies
- Prefer stdlib where reasonable
- All deps must be in go.mod (no vendoring)

## Linting
- golangci-lint with config in .golangci.yml
- Enabled linters: errcheck, govet, staticcheck, gosimple, ineffassign, unused

## Agent Teams Convention
- Each agent commits with scope prefix matching their role:
  feat(collector): ..., feat(api): ..., test(collector): ...
- Agents must NOT modify files outside their owned directories
- Shared interfaces live in internal/collector/collector.go — changes
  require Team Lead coordination
