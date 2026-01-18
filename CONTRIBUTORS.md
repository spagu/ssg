# Contributors

Thank you to everyone who has contributed to SSG!

## Maintainers

- **spagu** - Creator and lead maintainer

## How to Contribute

We welcome contributions! Here's how you can help:

### Reporting Bugs

1. Check existing issues first
2. Use the bug report template
3. Include reproduction steps
4. Mention your OS and SSG version

### Feature Requests

1. Open a discussion first
2. Explain the use case
3. Propose a solution if possible

### Pull Requests

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Write tests for new code
4. Ensure all tests pass (`go test ./...`)
5. Run linter (`golangci-lint run`)
6. Commit with clear messages
7. Open a Pull Request

### Development Setup

```bash
# Clone the repo
git clone https://github.com/spagu/ssg.git
cd ssg

# Install dependencies
go mod download

# Build
make build

# Run tests
make test

# Run linter
make lint
```

### Code Style

- Follow Go conventions
- Use `gofmt` for formatting
- Document exported functions
- Write meaningful commit messages

### Areas for Contribution

- 🐛 Bug fixes
- 📝 Documentation improvements
- 🧪 Test coverage
- 🌍 Translations
- 🎨 Template designs
- 🔧 Template engine improvements
- 📦 Package manager support

## Recognition

Contributors are recognized in:
- Release notes
- This file
- GitHub contributors page

## Code of Conduct

Please follow our [Code of Conduct](CODE_OF_CONDUCT.md) in all interactions.

---

*Want to be listed here? Submit a pull request!*
