# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build, Run, and Test

```makefile
make build    # Build binary to ./bin/exchange
make run     # Run the binary
make test    # Run all tests with verbose output
```

## Project Overview

This is a Go-based crypto exchange backend. The project is in early development with only a minimal main.go skeleton.

## Architecture

The application uses Go modules (`github.com/umikok7/crypto-exchange`). All source code lives at the root level for now — as the project grows, backend services will likely be organized under `/internal` or `/cmd`, with domain logic separated by feature (e.g., `orderbook`, `matching`, `trading`).

## Go Version

Go 1.25.0