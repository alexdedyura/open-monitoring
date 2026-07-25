<p align="center"><img src="assets/logo.svg" width="96" alt="Open Monitoring logo"></p>

# Open Monitoring

Open-source PC monitoring for Windows — an alternative to MSI Afterburner /
FPS Monitor. Live dashboard, recordable sessions (up to 4 hours) with zoomable
charts, and a configurable always-on-top HUD overlay.

**Stack:** Go + [Wails v2](https://wails.io) · Svelte 5 + Tailwind CSS 4 · [uPlot](https://github.com/leeoniya/uPlot)

One build produces **one executable** — the native helpers are compiled into it
and unpacked at runtime, and nothing has to be installed on the target machine.

**Documentation:** [alexdedyura.github.io/open-monitoring](https://alexdedyura.github.io/open-monitoring/) —
built with [Blume](https://github.com/haydenbleasel/blume) from `docs/`.

## Features

- **Live dashboard** — CPU (total + per-core), RAM, page file (swap) usage and
  growth, GPU, VRAM, disk I/O and network, streaming charts with 1m/5m/15m/30m
  windows. Dark and light themes (both chart palettes are
  colorblind-validated). The interface follows the Windows display scaling by
  default, or can be pinned to 100–200%.
- **GPU telemetry for every vendor** — load, VRAM, temperature, hot spot,
  power draw, clocks and fan. NVIDIA via NVAPI, AMD via ADL, Intel via IGCL —
  no vendor CLI tool involved.
- **FPS of the foreground app** — current / average / 1% low / 0.1% low via
  [Intel PresentMon](https://github.com/GameTechDev/PresentMon) (ETW frame
  capture), shown in the HUD and recorded into sessions.
- **CPU clock** — real boost frequency of the fastest core, read from Windows
  performance counters, with no kernel driver needed.
- **CPU temperature and power** — read through the [PawnIO](https://pawnio.eu)
  driver, which the app requires: at first launch it offers a one-click
  install and stays locked until the driver is present; see
  [The PawnIO driver](#the-pawnio-driver).
- **Storage monitoring** — per-volume live read/write speeds and space usage,
  plus physical drives with model, bus (NVMe/SATA), media type, health status,
  and SMART temperature / remaining life / power-on hours, from the Windows
  storage WMI namespace.
- **System panel** — hardware summary with brand logos (Intel / AMD / NVIDIA
  via [simple-icons](https://github.com/simple-icons/simple-icons)): CPU with
  base clock, GPU, RAM modules (type/speed/vendor), page file, motherboard,
  OS with hostname and uptime, and drives.
- **Recording sessions** — capture every metric for 1–4 hours into SQLite
  (`%APPDATA%\OpenMonitoring\sessions.db`), then browse them as zoomable charts
  (drag to zoom, double-click to reset) or export to CSV.
- **HUD overlay** — one click turns the window into a compact, frameless,
  always-on-top overlay styled like a classic in-game OSD: titled sections per
  hardware (GPU / CPU / Memory / Storage / Network / FPS) with per-row toggles,
  live FPS and frame-time sparklines, opacity control and screen anchoring
  (free drag or any corner). Works over windowed and borderless-fullscreen
  games; exclusive fullscreen bypasses any desktop overlay — the app explains
  this before switching.
- The app requests **administrator elevation at launch** (manifest): SMART
  counters and ETW frame capture need it, same as MSI Afterburner.

## Download

Every push to the default branch is built by GitHub Actions and published to
the rolling [**latest** release](https://github.com/alexdedyura/open-monitoring/releases/tag/latest) —
grab `open-monitoring.exe` from there. One file, no prerequisites; the PawnIO
driver is offered on first launch.

## Building

Requirements: Go 1.25+, Node.js 18+, the
[Wails CLI](https://wails.io/docs/gettingstarted/installation) v2, and the
.NET 8 SDK (to compile the sensor helper — end users do not need .NET).

```powershell
.\build.ps1
```

That publishes the C# sensor helper, fetches PresentMon if it is not already in
`tools/presentmon/`, stages both into `internal/sidecar/bin/`, and runs
`wails build`. The result is `build/bin/open-monitoring.exe` — around 50 MB,
with no loose files beside it and no runtime prerequisites.

Use `.\build.ps1 -SkipHelpers` while iterating on Go or frontend code to reuse
the already-staged helpers.

For development with hot reload, `wails dev` works as usual; helpers are picked
up straight from `tools/` so there is no need to re-stage them. Note that the
manifest requests administrator elevation, so `wails dev` triggers a UAC prompt
on every rebuild — temporarily set `requestedExecutionLevel` to `asInvoker` in
`build/windows/wails.exe.manifest` for long dev sessions.

## Project layout

```
main.go                  Wails wiring, nothing else
internal/app/            the application: bindings, recording, window/HUD control
internal/metrics/        metric collection (see below)
internal/sidecar/        embeds the helper executables and unpacks them on demand
internal/store/          session persistence (SQLite)
tools/lhm-bridge/        the C# sensor helper
build.ps1                one-command build
```

Every exported method on `app.App` becomes a JavaScript binding, so that set is
effectively the app's API; `internal/app/bindings.go` collects the thin ones.

## How it works

The collector samples every source on a configurable interval (250 ms – 5 s),
keeps an in-memory ring buffer so charts open already populated, and pushes each
sample to the frontend as a Wails event. Recording batches samples into SQLite
transactions and auto-stops at the configured cap.

Four sources feed a sample, each covering what the others cannot:

| Source | Provides | Library |
|---|---|---|
| System | CPU load, memory, disk and network throughput | [gopsutil v4](https://github.com/shirou/gopsutil) (pure Go) |
| `lhm-bridge` | GPU telemetry (all vendors), CPU package temperature and power, motherboard and memory modules (SMBIOS), drive wear | [LibreHardwareMonitorLib](https://github.com/LibreHardwareMonitor/LibreHardwareMonitor) 0.9.6 (MPL-2.0) |
| PresentMon | frame times of the foreground app | [Intel PresentMon](https://github.com/GameTechDev/PresentMon) |
| WMI | drive identity and SMART counters, CPU boost clock | [yusufpapurcu/wmi](https://github.com/yusufpapurcu/wmi) |

Anything the sensor helper can answer is asked of it rather than of WMI —
SMBIOS needs no privileges and no second query path. WMI is left with the two
things the helper does not cover: physical-drive identity (bus, media type,
health, power-on hours) and a CPU clock that works when the driver below is
not installed.

Sessions are stored with [modernc.org/sqlite](https://gitlab.com/cznic/sqlite)
(pure Go, no cgo), and charts are drawn with
[uPlot](https://github.com/leeoniya/uPlot).

### Why there is a C# helper

LibreHardwareMonitor has no Go equivalent: it wraps NVAPI, AMD ADL, Intel IGCL,
SMBIOS and a kernel driver to reach sensors Windows exposes through no public
API. Reimplementing that is not realistic, so `tools/lhm-bridge` is a small C#
worker that streams one JSON reading per interval on stdout.

It is published **self-contained** and compiled into the Go binary with
`go:embed`, then unpacked to `%LOCALAPPDATA%\OpenMonitoring\bin\` on first use
under a content-hashed name. That is what keeps the distributable a single file
while still not requiring .NET on the machine that runs it. The same applies to
PresentMon.

The helper also owns all knowledge of *sensor names* — LibreHardwareMonitor
reports vendor-specific labels like `CPU Package`, `Core (Tctl/Tdie)` or
`GPU Hot Spot`, and choosing between them belongs next to the sensor objects
rather than in the Go consumer.

## The PawnIO driver

Reading CPU package temperature and power requires ring-0 access to the
processor's model specific registers. There is no driver-free source for them on
desktop hardware — the ACPI thermal zone some laptops expose reports "not
supported" there.

Historically LibreHardwareMonitor got that access through the **WinRing0**
driver, which Microsoft Defender has classified as a vulnerable driver since
March 2025 ([CVE-2020-14979](https://nvd.nist.gov/vuln/detail/CVE-2020-14979))
and quarantines on sight. LibreHardwareMonitorLib 0.9.6 dropped WinRing0
entirely in favour of [PawnIO](https://pawnio.eu), a separately installed
open-source (GPL-2.0) kernel driver that runs small sandboxed modules instead
of handing userland arbitrary MSR access. This app never installs a kernel
driver of its own.

**PawnIO is required.** When the sensor engine reports the driver missing, the
app shows an install gate and stays locked until it is present. The gate runs

```
winget install --id namazso.PawnIO --exact --silent
```

and then relaunches the sensor helper, because LibreHardwareMonitor decides
only once, at startup, whether the driver exists. Where winget is unavailable
the official download page is opened instead; the app never downloads or
executes an installer of its own. Once the driver reports in, the dashboard
opens by itself — no restart needed.

## Roadmap

- Global hotkey for HUD toggle
- Per-process metrics
- Russian localisation

## License

[GPL-3.0](LICENSE).
