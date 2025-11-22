# Contributing to NDC ClickHouse Go

Thank you for your interest in contributing! This document provides guidelines and information for contributors.

## Getting Started

1. **Fork the repository** on GitHub
2. **Clone your fork** locally:
   ```bash
   git clone https://github.com/YOUR_USERNAME/ndc-clickhouse-go.git
   cd ndc-clickhouse-go
   ```
3. **Set up development environment**:
   ```bash
   make deps
   docker-compose up -d clickhouse
   ```
4. **Create a branch** for your changes:
   ```bash
   git checkout -b feature/your-feature-name
   ```

## Development Workflow

### Before Making Changes

1. Check existing issues and PRs
2. For large changes, open an issue first to discuss
3. Make sure tests pass: `make test`

### Making Changes

1. Write code following our style guide
2. Add tests for new functionality
3. Update documentation if needed
4. Keep commits focused and atomic

### Testing

```bash
# Run all tests
make test

# Run specific test
go test -v ./schema/... -run TestParseClickHouseType

# Run integration tests (requires ClickHouse)
docker-compose up -d clickhouse
go test -tags=integration ./tests/...

# Run with coverage
make test-coverage
```

### Code Quality

```bash
# Format code
make fmt

# Run linter
make lint

# Both
make fmt && make lint
```

## Pull Request Process

1. **Update your branch** with the latest main:
   ```bash
   git fetch origin
   git rebase origin/main
   ```

2. **Run all checks**:
   ```bash
   make fmt
   make lint
   make test
   ```

3. **Push your branch**:
   ```bash
   git push origin feature/your-feature-name
   ```

4. **Create a Pull Request** on GitHub with:
   - Clear title describing the change
   - Description of what and why
   - Link to related issues
   - Screenshots if UI-related

5. **Address review feedback** by adding commits

6. **Squash and merge** once approved

## Code Style Guide

### Go Code

- Follow [Effective Go](https://golang.org/doc/effective_go.html)
- Use `gofmt` for formatting
- Keep functions focused and small
- Add comments for exported functions
- Handle errors explicitly

```go
// Good
func ProcessQuery(ctx context.Context, query string) (*Result, error) {
    if query == "" {
        return nil, errors.New("query cannot be empty")
    }
    // ...
}

// Bad
func process(q string) *Result {
    // Missing context, error handling, and documentation
}
```

### Error Handling

- Always wrap errors with context
- Use `fmt.Errorf` with `%w` for wrapping

```go
// Good
if err != nil {
    return nil, fmt.Errorf("failed to execute query: %w", err)
}

// Bad
if err != nil {
    return nil, err
}
```

### Testing

- Use table-driven tests
- Test edge cases
- Use descriptive test names

```go
func TestParseType(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        expected TypeInfo
    }{
        {
            name:     "simple string",
            input:    "String",
            expected: TypeInfo{BaseType: "String"},
        },
        // More cases...
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := ParseType(tt.input)
            if result != tt.expected {
                t.Errorf("got %v, want %v", result, tt.expected)
            }
        })
    }
}
```

## Types of Contributions

### Bug Reports

- Use the bug report template
- Include reproduction steps
- Provide environment details
- Include relevant logs

### Feature Requests

- Use the feature request template
- Explain the use case
- Describe expected behavior
- Consider implementation approach

### Documentation

- Fix typos and errors
- Add examples
- Improve clarity
- Add missing information

### Code

- Bug fixes
- New features
- Performance improvements
- Test coverage

## Project Areas

### Core Connector (`connector/`)

The main NDC interface implementation. Changes here affect all queries and mutations.

### Query Builder (`internal/query/`)

SQL generation from NDC queries. Critical for correctness and performance.

### Schema/Types (`schema/`)

Type mapping between ClickHouse and GraphQL. Important for schema accuracy.

### Configuration (`config/`)

Configuration parsing and validation. Affects user experience.

### CLI (`cmd/`)

Command-line tools. Focus on usability and helpful error messages.

## Communication

- **GitHub Issues**: Bug reports, feature requests
- **Pull Requests**: Code contributions
- **Discussions**: Questions and ideas

## License

By contributing, you agree that your contributions will be licensed under the Apache License 2.0.

## Recognition

Contributors are recognized in:
- Release notes
- CONTRIBUTORS.md file
- GitHub contributors page

Thank you for contributing!
