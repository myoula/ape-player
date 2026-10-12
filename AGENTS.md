# Repository Guidelines

## Project Structure & Module Organization

Ape Player is a Linux desktop Emby client using MyGO Native UI and libmpv with OpenGL rendering.

- `cmd/ape-player/`: executable entry point and diagnostic flags.
- `internal/app/`: UI, navigation, asynchronous requests, and playback queues.
- `internal/emby/`: HTTP API, playback negotiation, and progress reporting.
- `internal/player/`: purego bindings, rendering, and media controls.
- `internal/config/` and `internal/diagnostics/`: configuration persistence and sanitized logs.
- `internal/icons/` and `assets/`: bundled Lucide icons and desktop assets.
- Tests live beside implementation in `*_test.go`; `scripts/` contains installation and smoke-test helpers.
- `third_party/mygo/` is the active local framework replacement. Read its `AGENTS.md` before editing and document framework changes in `third_party/MYGO-PATCHES.md`.

## Build, Test, and Development Commands

Use Go 1.27.1+; running requires GTK 3, libepoxy, OpenGL, and `libmpv.so.2`. Keep builds compatible with `CGO_ENABLED=0`.

- `make build`: build `build/ape-player` and copy icon licensing.
- `./build/ape-player`: launch the desktop application.
- `make test`: run Go package tests.
- `make vet`: run Go vet and MyGO UI checks.
- `make test vet build`: complete standard validation.
- `./build/ape-player --play /path/to/video.mkv`: check local playback.

## Coding Style & Naming Conventions

Format Go changes with `gofmt`; use its tab indentation and standard Go naming conventions. Keep packages lowercase, exported identifiers in PascalCase, and internal identifiers in camelCase. Follow neighboring file organization. Keep UI text in Chinese and icon buttons accessible through labels and tooltips. Preserve native rendering and direct playback without requiring server transcoding.

## Testing Guidelines

Use Go's `testing` package, `httptest` for Emby fixtures, and MyGO `ui.NewTester` for UI interactions. Name tests `TestBehavior` in adjacent `*_test.go` files. Update relevant behavioral tests; no numeric coverage threshold is configured.

Playback/rendering changes also need native GPU validation using the fixture and commands in `README.md`. Record material results and limitations in `VALIDATION.md`; headless tests do not establish real-server compatibility.

## Commit & Pull Request Guidelines

This workspace has no root Git history, so no existing commit convention can be verified. Use concise imperative subjects, such as `Fix synchronized player controls`. Describe the problem, behavior changes, and validation in PRs. Link relevant issues and include screenshots for visible UI changes.

## Security & Configuration

Never commit credentials or dump real configuration. XDG configuration stores access tokens with `0600` permissions; passwords must not persist. Sanitize playback URLs and logs. Real-server `--diagnose` runs report playback progress, so prefer local fixtures for routine checks.
