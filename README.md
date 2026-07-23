# Open Monitoring

Open-source PC monitoring for Windows — an alternative to MSI Afterburner / FPS Monitor.
Live dashboard, recordable sessions (up to 4 hours) with zoomable charts, and a
configurable always-on-top HUD overlay.

**Stack:** Go + [Wails v2](https://wails.io) · Svelte 5 + Tailwind CSS 4 · [uPlot](https://github.com/leeoniya/uPlot)

## Features

- **Live dashboard** — CPU (total + per-core), RAM, GPU, VRAM, disk I/O and
  network, streaming charts with 1m/5m/15m/30m windows. Dark and light themes
  (both chart palettes are colorblind-validated), interface scaling
  (100–200%).
- **FPS of the foreground app** — current / average / 1% low / 0.1% low via
  [Intel PresentMon](https://github.com/GameTechDev/PresentMon) (ETW frame
  capture), shown in the HUD and recorded into sessions.
- **GPU telemetry** — usage, VRAM, temperature, power draw, clocks and fan via a
  single long-lived `nvidia-smi` stream (NVIDIA). AMD/Intel GPUs are picked up
  through the embedded sensor engine.
- **Embedded LibreHardwareMonitor** — a bundled `lhm-bridge.exe` (tiny C# worker
  over [LibreHardwareMonitorLib](https://github.com/LibreHardwareMonitor/LibreHardwareMonitor),
  MPL-2.0) is spawned automatically and streams CPU temperature, package power,
  clocks, fans and drive SMART data. An external LHM web server
  (`http://localhost:8085`) is used as fallback when the bridge binary is
  missing. The app requests **administrator elevation at launch** (manifest) —
  kernel-level sensors and ETW capture need it, same as MSI Afterburner.
- **Storage monitoring** — per-volume live read/write speeds and space usage,
  plus physical drives with model, bus (NVMe/SATA), media type, WMI health
  status and SMART temperature / remaining life.
- **System panel** — hardware summary with brand logos (Intel / AMD / NVIDIA
  via [simple-icons](https://github.com/simple-icons/simple-icons)): CPU,
  GPU, RAM modules (type/speed/vendor), motherboard, OS and drives.
- **Recording sessions** — capture every metric for 1–4 hours into SQLite
  (`%APPDATA%\OpenMonitoring\sessions.db`), then browse them as zoomable charts
  (drag to zoom, double-click to reset) or export to CSV.
- **HUD overlay** — one click turns the window into a compact, frameless,
  always-on-top overlay styled like a classic in-game OSD: titled sections per
  hardware (GPU / CPU / Memory / Storage / Network / FPS) with per-row toggles,
  opacity control and screen anchoring (free drag or any corner). Works over
  windowed / borderless-fullscreen games; exclusive fullscreen bypasses any
  desktop overlay — the app explains this before switching.

## Building

Requirements: Go 1.24+, Node.js 18+,
[Wails CLI](https://wails.io/docs/gettingstarted/installation) v2, and the
.NET 8 SDK (for the sensor bridge).

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest
dotnet publish tools/lhm-bridge -c Release -o tools/lhm-bridge/publish
# FPS support: download the console app from the official PresentMon releases
# https://github.com/GameTechDev/PresentMon/releases (PresentMon-<ver>-x64.exe)
# and save it as tools/presentmon/PresentMon.exe
wails build
cp tools/lhm-bridge/publish/lhm-bridge.exe build/bin/
cp tools/presentmon/PresentMon.exe build/bin/
```

The binary lands in `build/bin/open-monitoring.exe`; keep `lhm-bridge.exe` and
`PresentMon.exe` next to it (end users only need the .NET 8 runtime for the
bridge — the app itself works without them, minus the respective sensors). For
development with hot reload: `wails dev` (helpers are picked up from `tools/`).
Note: the manifest requests administrator elevation, so `wails dev` triggers a
UAC prompt on every rebuild; temporarily set `requestedExecutionLevel` to
`asInvoker` in `build/windows/wails.exe.manifest` for long dev sessions.

## How it works

| Concern | Library |
|---|---|
| CPU / RAM / disk / network | [gopsutil v4](https://github.com/shirou/gopsutil) |
| NVIDIA GPU | `nvidia-smi --query-gpu … -lms 1000` (one streaming process) |
| Temperatures, fans, SMART, non-NVIDIA GPUs | embedded `lhm-bridge` on LibreHardwareMonitorLib (HTTP endpoint as fallback) |
| Drive models / bus / health, board, RAM modules | WMI (`MSFT_PhysicalDisk`, `Win32_BaseBoard`, `Win32_PhysicalMemory`) |
| FPS / frame times of the foreground app | [Intel PresentMon](https://github.com/GameTechDev/PresentMon) (`--output_stdout` stream) |
| Session storage | [modernc.org/sqlite](https://gitlab.com/cznic/sqlite) (pure Go, no cgo) |
| Charts | [uPlot](https://github.com/leeoniya/uPlot) |
| Brand logos | [simple-icons](https://github.com/simple-icons/simple-icons) |
| Desktop shell / bindings | [Wails v2](https://wails.io) |

The Go collector samples all sources on a configurable interval (250 ms – 5 s),
keeps an in-memory ring buffer for chart hydration and pushes each sample to the
frontend as a Wails event. Recording batches samples into SQLite transactions
and auto-stops at the configured cap.

## Roadmap

- Global hotkey for HUD toggle
- Per-process metrics
- Frame-time graph in the HUD

## License

See [LICENSE](LICENSE).
