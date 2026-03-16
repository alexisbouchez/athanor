# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Athanor is a simple CI engine written in Go that allows self-hosting GitHub Actions using CloudHypervisor microVMs. The project prioritizes simplicity.

## Build & Development Commands

```bash
make build              # Build binary to bin/athanor
make test               # Run all tests
make vet                # Static analysis
make build-linux        # Cross-compile for Linux (arm64 + amd64)
go test ./internal/vmm -run TestGetVMInfo  # Run a single test
```

## Lima VM Management

CloudHypervisor requires KVM, so on macOS we run it inside a Lima VM with nested virtualization.

```bash
make lima-create        # Create the Lima VM from lima/athanor.yaml
make lima-start         # Start the VM
make lima-shell         # Shell into the VM
make lima-stop          # Stop the VM
make lima-delete        # Delete the VM
```

## Architecture

The high-level flow: GitHub webhook → job scheduling → microVM provisioning (CloudHypervisor) → action execution → result reporting back to GitHub.

Key packages:
- `cmd/athanor/` — CLI entrypoint
- `internal/vmm/` — cloud-hypervisor API monitor (talks to the CH HTTP API over Unix socket)

## License

GPLv3
