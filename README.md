# Open Monitoring

Open-source PC monitoring for Windows — an alternative to MSI Afterburner / FPS Monitor.
Live dashboard, recordable sessions (up to 4 hours) with zoomable charts, and a
configurable always-on-top HUD overlay.

**Stack:** Go + [Wails v2](https://wails.io) · Svelte 5 + Tailwind CSS 4 · [uPlot](https://github.com/leeoniya/uPlot)

## Features

- **Live dashboard** — CPU (total + per-core), RAM, GPU, VRAM, disk I/O and
  network, streaming charts with 1m/5m/15m/30m windows.
- **GPU telemetry** — usage, VRAM, temperature, power draw, clocks and fan via a
  single long-lived `nvidia-smi` stream (NVIDIA). AMD/Intel GPUs are picked up
  through LibreHardwareMonitor when it is running.
- **CPU temperature / package power / clocks** — optional integration with
  [LibreHardwareMonitor](https://github.com/LibreHardwareMonitor/LibreHardwareMonitor):
  start it with *Options → Remote Web Server* enabled and the rows light up.
  No kernel driver ships with this app.
- **Recording sessions** — capture every metric for 1–4 hours into SQLite
  (`%APPDATA%\OpenMonitoring\sessions.db`), then browse them as zoomable charts
  (drag to zoom, double-click to reset) or export to CSV.
- **HUD overlay** — one click turns the window into a compact, frameless,
  always-on-top overlay. Rows, order and opacity are configurable; position and
  size are remembered. Works over windowed / borderless-fullscreen games.

## Building

Requirements: Go 1.24+, Node.js 18+, [Wails CLI](https://wails.io/docs/gettingstarted/installation) v2.

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest
wails build
```

The binary lands in `build/bin/open-monitoring.exe`. For development with hot
reload: `wails dev`.

## How it works

| Concern | Library |
|---|---|
| CPU / RAM / disk / network | [gopsutil v4](https://github.com/shirou/gopsutil) |
| NVIDIA GPU | `nvidia-smi --query-gpu … -lms 1000` (one streaming process) |
| Temperatures, fans, non-NVIDIA GPUs | LibreHardwareMonitor JSON endpoint (optional) |
| Session storage | [modernc.org/sqlite](https://gitlab.com/cznic/sqlite) (pure Go, no cgo) |
| Charts | [uPlot](https://github.com/leeoniya/uPlot) |
| Desktop shell / bindings | [Wails v2](https://wails.io) |

The Go collector samples all sources on a configurable interval (250 ms – 5 s),
keeps an in-memory ring buffer for chart hydration and pushes each sample to the
frontend as a Wails event. Recording batches samples into SQLite transactions
and auto-stops at the configured cap.

## Roadmap

- FPS overlay via [PresentMon](https://github.com/GameTechDev/PresentMon)
  (ETW frame-time capture)
- Global hotkey for HUD toggle
- Per-process metrics

## License

See [LICENSE](LICENSE).
