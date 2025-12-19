# Contributing to otlpxy

Thank you for your interest in contributing to otlpxy! We welcome contributions from the community.

## How to Contribute

### Reporting Bugs

If you find a bug, please create an issue with:
- A clear, descriptive title
- Steps to reproduce the issue
- Expected behavior vs actual behavior
- Your environment (Go version, OS, etc.)
- Any relevant logs or error messages

### Suggesting Enhancements

We welcome feature requests! Please create an issue with:
- A clear description of the feature
- The problem it solves
- Any alternative solutions you've considered
- Examples of how it would be used

### Pull Requests

1. **Fork the repository** and create your branch from `main`
2. **Write clear commit messages** following conventional commits format:
   - `feat: add new feature`
   - `fix: resolve bug in worker pool`
   - `docs: update README`
   - `refactor: improve error handling`
   - `test: add tests for CORS middleware`
3. **Add tests** for new functionality
4. **Update documentation** if needed
5. **Ensure tests pass**: `make test`
6. **Format your code**: `go fmt ./...`
7. **Submit the PR** with a clear description

### Development Setup

```bash
# Clone your fork
git clone https://github.com/YOUR_USERNAME/otlpxy.git
cd otlpxy

# Install dependencies
go mod download

# Run tests
make test

# Build
make build

# Run locally
make run
```

### Code Style

- Follow standard Go conventions
- Use `gofmt` for formatting
- Write clear, self-documenting code
- Add comments for complex logic
- Keep functions focused and testable

### Testing

- Write unit tests for new functionality
- Ensure existing tests pass
- Aim for meaningful test coverage
- Test edge cases and error conditions

### Commit Guidelines

- Keep commits atomic and focused
- Write clear, concise commit messages
- Reference issues in commits (e.g., `fixes #123`)

## Code of Conduct

### Our Pledge

We are committed to providing a welcoming and inclusive environment for everyone.

### Our Standards

- Be respectful and considerate
- Welcome diverse perspectives
- Accept constructive criticism gracefully
- Focus on what's best for the community
- Show empathy towards others

### Unacceptable Behavior

- Harassment or discriminatory language
- Trolling or insulting comments
- Personal or political attacks
- Publishing others' private information
- Other unprofessional conduct

## Questions?

Feel free to open an issue for any questions about contributing!
