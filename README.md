# Privateer SDK

The **Privateer SDK** provides the interface and utilities needed for developing Privateer plugins. It includes common logic, cloud provider utilities, and an evaluation framework that can be reused across multiple plugins.

## Documentation

**For complete SDK documentation, visit [privateerproj.com/docs/developers/sdk/](https://privateerproj.com/docs/developers/sdk/)**

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

See the [plugin development guide](https://privateerproj.com/docs/developers/plugins/) for detailed usage examples.

## API Reference

- **[pkg.go.dev Documentation](https://pkg.go.dev/github.com/privateerproj/privateer-sdk)** - Complete API reference
- **[SDK Documentation](https://privateerproj.com/docs/developers/sdk/)** - Developer guide and tutorials

## Local Development

### Prerequisites

- **Go 1.25.1 or later** - Required for building and testing
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

```
privateer-sdk/
├── command/        # CLI command utilities
├── config/         # Configuration management
├── pluginkit/      # Core plugin kit functionality
├── shared/         # Shared plugin interfaces
└── utils/          # Utility functions
```

## Contributing

We welcome contributions! See the [Privateer contributing guidelines](https://github.com/privateerproj/privateer?tab=contributing-ov-file) for details.

All contributions are covered by the [Apache 2 License](LICENSE).

## License

Licensed under the Apache License, Version 2.0. See [LICENSE](LICENSE) for details.

## Related Projects

- **[Privateer Core](https://github.com/privateerproj/privateer)** - The main Privateer CLI
- **[Privateer Documentation](https://privateerproj.com)** - Complete documentation site
- **[Example Plugin](https://github.com/privateerproj/raid-wireframe)** - Reference implementation
