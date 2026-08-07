# desktop

[![ci](https://github.com/go-widgets/desktop/actions/workflows/ci.yml/badge.svg)](https://github.com/go-widgets/desktop/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/go-widgets/desktop.svg)](https://pkg.go.dev/github.com/go-widgets/desktop)
[![Go Report Card](https://goreportcard.com/badge/github.com/go-widgets/desktop)](https://goreportcard.com/report/github.com/go-widgets/desktop)
![Go 1.26.4](https://img.shields.io/badge/go-1.26.4-00ADD8?logo=go)
![License BSD-3-Clause](https://img.shields.io/badge/license-BSD--3--Clause-blue)

A native, pure-Go (CGO=0) **desktop-shell demo** that composes the whole
`go-freedesktop` + `go-widgets` stack into one runnable app. Each freedesktop
library does its real job against a real Linux filesystem, and the UI is drawn
entirely with go-widgets widgets and box layouts.

## What each library drives

| Feature | Library | What it does here |
|---|---|---|
| Dock + launcher | [`go-freedesktop/desktopentry`](https://github.com/go-freedesktop/desktopentry) | `Scan()` → the app index; `ExpandExec` → launch argv |
| Icons | [`go-freedesktop/icontheme`](https://github.com/go-freedesktop/icontheme) | `FindIcon` → PNG/JPEG rasterized into go-widgets `Image` |
| Application menu | [`go-freedesktop/menu`](https://github.com/go-freedesktop/menu) | `Load()` → a categorized tree rendered as a go-widgets `MenuBar`/`Menu` |
| "Open with" | [`go-freedesktop/mime`](https://github.com/go-freedesktop/mime) + [`mimeapps`](https://github.com/go-freedesktop/mimeapps) | `TypeByNameAndContent` + `Candidates`/`DefaultApp` |
| File grid + thumbnails | [`toolkit/virtual`](https://github.com/go-widgets/toolkit) `VirtualGrid` + [`go-thumbnail`](https://github.com/go-thumbnail/thumbnail) | virtualized gengrid over a directory, thumbnail cache keys per cell |
| Notifications | [`go-freedesktop/notifications`](https://github.com/go-freedesktop/notifications) | a live `org.freedesktop.Notifications` daemon → go-widgets `Toast`s |

## Architecture

Composition/model logic is separated from raw rendering:

- **`shell/`** — pure, side-effect-light model logic with **100% statement
  coverage** (including error branches): the launchable-app index, MIME
  "open with" resolution, the categorized menu model, the directory listing
  model and the thumbnail cache-key policy. No rendering surface, so every
  branch is unit-coverable against temp-dir fixtures.
- **`render/`** — turns those models into a go-widgets widget tree. The shell is
  a `Border` layout (menubar north, dock south, launcher west, file grid center)
  plus a floating `Toast` stack. State flows through `go-widgets/mvvm`
  (`Observable` search query → `ObservableList` results → the launcher).
- **`cmd/desktop/`** — the native binary: scans the filesystem, composes the
  scene and renders it.

go-widgets is a pure pixel-blitting toolkit, so the shell renders into an
offscreen framebuffer; a host compositor presents it, and `-capture` writes it
to a PNG — the headless "screenshot" path.

## Usage

```sh
go run ./cmd/desktop -dir ~/Pictures -capture shell.png     # render to PNG
go run ./cmd/desktop -query fire                            # seed launcher filter
go run ./cmd/desktop -launch org.mozilla.firefox            # expand Exec + launch

# Notifications need a session bus (Linux):
dbus-run-session -- go run ./cmd/desktop -notify "Build|Finished" -capture toast.png
```

## License

BSD-3-Clause. Copyright (c) 2026 the go-widgets/desktop authors.
