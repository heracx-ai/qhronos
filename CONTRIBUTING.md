# Contributing to Qhronos

Thank you for your interest in contributing to Qhronos! We welcome pull requests and community involvement.

## How to Contribute

1. **Fork the repository** and create your branch from `main`.
2. **Make your changes** with clear, descriptive commit messages.
3. **Add tests** for any new features or bug fixes.
4. **Run all tests** to ensure nothing is broken:
   ```sh
   make test
   ```
5. **Submit a pull request (PR)** with a clear description of your changes and why they are needed.

## Code Style
- Follow idiomatic Go best practices: [Effective Go](https://golang.org/doc/effective_go.html)
- Use `gofmt` or `go fmt` to format your code.
- Keep functions small and focused.
- Write clear, descriptive comments for exported functions and complex logic.

## Testing
- All new features and bug fixes should include tests.
- Use `make test` to run the full test suite.
- If your change requires database or Redis, ensure Docker containers are running (`make docker-up`).

## Project Structure
- Main application code is in `main.go` and `internal/`.
- Configuration examples are in `config.example.yaml`.
- Migrations are in `migrations/` and are embedded in the binary.

## Pull Request Process
- Ensure your branch is up to date with `main` before submitting a PR.
- Describe your changes and reference any related issues.
- One feature or fix per PR is preferred.
- The maintainers will review your PR and may request changes.

## Community
- Be respectful and constructive in all communications.
- We welcome suggestions, bug reports, and feature requests via GitHub Issues.

Thank you for helping make Qhronos better! 