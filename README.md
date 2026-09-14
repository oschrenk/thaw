# thaw

Preview what an update would bring, before anything writes a lock.

`thaw` reads a flake, resolves its lock as a graph, and reports what would
change if its inputs moved. It writes no `flake.lock` and deploys nothing.

## Usage

```text
thaw [subcommand] [flake]

  packages    which package versions change, appear and disappear
  options     which NixOS options appear and disappear

  --rev-only           report which input revisions moved, and evaluate nothing
  --input <name>       report only what comes from this input
  --for <name>         report only this subject: a host, devshell, a package
  --all                report the whole build closure, build tools included
  --to <input>=<ref>   compare one input against a ref instead of upstream HEAD
  --releases           look up GitHub releases for the hand-pinned packages
  --json               machine-readable output
  --check              exit non-zero when anything moves
```

Bare `thaw` runs `packages --rev-only`. The flake argument defaults to `.`.

## Install

```sh
nix run github:oschrenk/thaw
```

## Build

```sh
task build
```

## Develop

`direnv allow` enters the devShell, which carries `go`, `golangci-lint` and
`gopls`.
