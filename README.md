# athanor

The simplest way to self-host GitHub Actions. Athanor parses your `.github/workflows/` YAML files and runs them locally with a terminal UI, so you can test workflows without pushing.

## Quick Start

```bash
# Build
make build

# Run the TUI — pick a workflow interactively
make run

# Run a specific workflow directly
make run-workflow WORKFLOW=.github/workflows/self-test.yml
```

Requires **Go 1.25+** and **bash**.

## What It Does

Athanor reads standard GitHub Actions workflow YAML and executes the `run:` steps on your machine. A split-panel TUI shows a job/step tree on the left and live log output on the right.

Supported workflow features:

| Feature | Example |
|---|---|
| `run:` steps | `run: go test ./...` |
| Job dependencies | `needs: [lint, test]` |
| Workflow/job/step `env:` | `env: { FOO: bar }` |
| Step outputs | `echo "key=val" >> "$GITHUB_OUTPUT"` |
| Output expressions | `${{ steps.id.outputs.key }}` |
| Conditions | `if: success()`, `failure()`, `always()` |
| `continue-on-error` | Step failure doesn't fail the job |
| `working-directory:` | Per-step or via job `defaults:` |
| `shell:` | Per-step or via job `defaults:` |

Not yet supported: `uses:` actions, matrix strategy, containers/services, reusable workflows, `$GITHUB_ENV`, `$GITHUB_PATH`, full expression evaluation.

## Example Workflows

The repo ships with two workflows you can try immediately:

**`.github/workflows/self-test.yml`** — runs athanor's own vet, test, and build as a DAG:

```
vet ──┬── test
      └── build
```

**`.github/workflows/demo.yml`** — demonstrates env vars, step outputs, `continue-on-error`, and `if: always()`.

## Architecture

```
cmd/athanor/          CLI entrypoint, wires TUI ↔ runner
internal/
  workflow/            YAML parser and types
  runner/              Execution engine (DAG sort, job/step orchestration)
  tui/                 Bubbletea terminal UI (selection + execution views)
  vmm/                 Cloud-hypervisor API monitor (future: run jobs in microVMs)
```

The runner sends events on a Go channel. The CLI goroutine forwards them as `tea.Msg` to the TUI. The runner has zero TUI knowledge; the TUI has zero execution logic. Tests can drain events without a TUI.

Jobs are topologically sorted with Kahn's algorithm. Jobs at the same depth in the DAG run concurrently.

## CLI Flags

```
--workflow-dir DIR    Workflow directory (default: .github/workflows)
--workflow FILE       Run a specific workflow file, skip selection
```

## Development

```bash
make build            # Build to bin/athanor
make test             # Run all tests
make vet              # Static analysis
make clean            # Remove build artifacts
```

### MicroVM Support (macOS)

CloudHypervisor requires KVM. On macOS, use a Lima VM with nested virtualization:

```bash
make lima-create      # Create VM from lima/athanor.yaml
make lima-start       # Start the VM
make lima-shell       # Shell into it
make lima-stop        # Stop the VM
make lima-delete      # Delete the VM
make build-linux      # Cross-compile for Linux (arm64 + amd64)
```

## License

GPLv3
