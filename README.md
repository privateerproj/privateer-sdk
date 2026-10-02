# Privateer SDK

[![OSPS Baseline](https://github.com/privateerproj/privateer-sdk/actions/workflows/osps-security-assessment.yml/badge.svg)](https://github.com/privateerproj/privateer-sdk/actions/workflows/osps-security-assessment.yml)
[![OpenSSF Best Practices](https://www.bestpractices.dev/projects/12018/baseline)](https://www.bestpractices.dev/projects/12018/baseline-1)

The **Privateer SDK** provides the interface and utilities needed for developing Privateer plugins. It includes common logic, cloud provider utilities, and an evaluation framework that can be reused across multiple plugins.

## Documentation

**For complete SDK documentation, visit [privateerproj.com/developer-reference/](https://privateerproj.com/developer-reference/)**

The website includes:

- Detailed SDK overview and components
- Plugin development guides
- API reference and examples
- Best practices and patterns

## Quick Start

### Installation

Add the SDK to your Go project:

```bash
go get github.com/privateerproj/privateer-sdk
```

### Usage

Import the SDK in your plugin:

```go
import (
    "github.com/privateerproj/privateer-sdk/pluginkit"
    "github.com/privateerproj/privateer-sdk/config"
    "github.com/privateerproj/privateer-sdk/shared"
)
```

See the [plugin development guide](https://privateerproj.com/developer-reference/build-a-plugin/) for detailed usage examples.

## API Reference

- **[pkg.go.dev Documentation](https://pkg.go.dev/github.com/privateerproj/privateer-sdk)** - Complete API reference
- **[SDK Documentation](https://privateerproj.com/developer-reference/)** - Developer guide and tutorials

## Local Development

### Prerequisites

- **Go 1.26.2 or later** - Required for building and testing
- **Make** - For using the Makefile build targets

### Building

```bash
make build
```

### Testing

Run all tests:

```bash
make test
```

Run tests with coverage:

```bash
make testcov
```

### Available Make Targets

- `make build` - Build all packages
- `make test` - Run tests and vet checks
- `make testcov` - Run tests with coverage report
- `make tidy` - Clean up go.mod dependencies
- `make quick` - Alias for `make build`

## Project Structure

```bash
privateer-sdk/
├── command/        # CLI command utilities
├── config/         # Configuration management
├── pluginkit/      # Core plugin kit functionality
├── shared/         # Shared plugin interfaces
└── utils/          # Utility functions
```

## Contributing

We welcome contributions! See our [Contributing Guidelines](https://github.com/privateerproj/.github/blob/main/.github/CONTRIBUTING.md) for details.

All contributions are covered by the [Apache 2 License](https://github.com/privateerproj/privateer-sdk?tab=Apache-2.0-1-ov-file) at the time the pull request is opened, and all community interactions are governed by our [Code of Conduct](https://github.com/privateerproj/.github/blob/main/.github/CODE_OF_CONDUCT.md).

## Security

For vulnerability reporting, please reference our [Security Policy](https://github.com/privateerproj/.github/blob/main/.github/SECURITY.md). Please do not open public issues for security vulnerabilities or questions. Report them privately through [GitHub private vulnerability reporting](https://github.com/privateerproj/privateer-sdk/security/advisories/new) instead.

See [CRA-READINESS.md](CRA-READINESS.md) for how the project voluntarily documents its security practices against the EU Cyber Resilience Act (CRA) readiness checklist.

## Helpful Links

- **[Privateer SDK](https://github.com/privateerproj/privateer-sdk)** - SDK for developing Privateer plugins
- **[Privateer Documentation](https://privateerproj.com)** - Complete documentation site
- **[Example Plugin](https://github.com/privateerproj/raid-wireframe)** - Reference implementation
