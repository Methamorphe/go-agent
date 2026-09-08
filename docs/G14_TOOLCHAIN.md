# G14 desktop toolchain lock

G14 deliberately targets **Wails v3** and does not carry a Wails v2 fallback.

## Pinned toolchain

- Go language/module baseline: `go 1.27`
- CI Go toolchain: `1.27.1`
- Wails framework and CLI: `v3.0.0-beta.18`
- Wails frontend runtime: `@wailsio/runtime@3.0.0-beta.18`
- CI Node.js: `24.19.0`
- CI npm: `11.17.0`
- TypeScript: `6.0.2`
- Vite: `8.2.2`
- Vitest: `5.0.0`

All frontend package versions in `package.json` are exact versions rather than ranges. The Go module graph is checked by CI with `go mod tidy` followed by a clean-tree assertion.

## Upgrade policy

A Wails bump is an explicit G14 maintenance change, never an ambient dependency update.

For every bump:

1. read the Wails release notes and breaking-change notes;
2. change the Go module and `@wailsio/runtime` to the **same exact Wails v3 release**;
3. regenerate bindings with that exact CLI version;
4. run frontend unit tests and production typecheck/build;
5. run Go tests, race tests and vet;
6. build the stripped desktop binary on Linux, macOS and Windows;
7. run the native startup smoke on all three platforms;
8. check binary/frontend size budgets and startup metrics for regressions;
9. inspect bindings, events and window APIs touched by the release;
10. only then accept the new pin.

If an upstream Wails v3 release regresses the desktop client, revert to the previous known-good **Wails v3** pin. Do not introduce a Wails v2 compatibility path.

## Reproduction

From `cmd/go-agent-gui`:

```sh
go run github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.18 generate bindings -ts -clean=true -d frontend/bindings .
cd frontend
npm install --no-audit --no-fund
npm test
npm run build
```

Then from the repository root:

```sh
go test ./...
go vet ./...
go build -tags production -trimpath -buildvcs=false -ldflags="-w -s" ./cmd/go-agent-gui
```

The CI workflow is the canonical cross-platform reproduction of the G14 exit gates.
