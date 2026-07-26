# CLAUDE.md

Open Monitoring — Windows PC-monitoring desktop app (MSI Afterburner / FPS
Monitor alternative). Wails v2 (Go) + Svelte 5 runes + Tailwind CSS 4 + uPlot.
Windows is the only supported target; `!windows` files are no-op stubs that
keep the module compiling elsewhere.

## Commands

```powershell
.\build.ps1                # full build → build/bin/open-monitoring.exe (single file)
.\build.ps1 -Installer     # + NSIS installer (needs makensis; PawnIO = default-on component)
.\build.ps1 -SkipHelpers   # reuse already-staged helpers (fast Go/frontend iteration)
wails dev                  # hot reload; bindings dev server at localhost:34115
go vet ./... ; go test ./internal/...
npm run build              # in frontend/ — Vite production build
```

- `go test -race` does not work here (no gcc for cgo).
- The manifest is `requireAdministrator`, so `wails dev` triggers a UAC prompt
  on every rebuild — flip `requestedExecutionLevel` to `asInvoker` in
  `build/windows/wails.exe.manifest` for long dev sessions.
- PowerShell: do not append `2>&1` to `wails build` / native commands — PS
  wraps stderr in NativeCommandError and reports failure on exit 0.

## Layout

```
main.go                  Wails wiring only
internal/app/            App struct; bindings.go (thin accessors), recording.go,
                         pawnio.go (driver install flow), window*.go (HUD topmost)
internal/config/         JSON settings, versioned migrate + Clamp validation
internal/metrics/        collector.go merges four sources (below)
internal/sidecar/        go:embed of helper .exes, content-hashed unpack
internal/store/          SQLite sessions (modernc.org/sqlite, no cgo)
internal/stress/         burn-in: one job per subsystem, cpu*.go (+ amd64 asm),
                         ram.go, disk*.go, gpu/opencl_windows.go
tools/lhm-bridge/        C# sensor helper (LibreHardwareMonitorLib 0.9.6)
frontend/src/lib/        state.svelte.js (shared $state), metricDefs.js (labels,
                         palette, formatters), chartDefs.js (charts + buffers)
```

Every exported method on `app.App` becomes a frontend binding — Wails generates
`frontend/wailsjs/go/app/App.js` (note `go/app/`, not `go/main/`); only
`state.svelte.js` imports it.

## Metric sources (four, no overlap)

- **gopsutil v4** (`system.go`) — CPU load, RAM, disk I/O, network.
- **lhm-bridge** (`sensors.go`) — GPU for all vendors (NVAPI/ADL/IGCL), CPU
  package temp+power, SMBIOS board/RAM. One typed JSON line per 2 s on stdout;
  all sensor-NAME knowledge lives on the C# side.
- **PresentMon** (`fps_windows.go`) — frame times, needs elevation.
- **WMI** (`sysinfo/smart/cpuclock_windows.go`) — drive identity + SMART, and
  the driver-free CPU boost clock (fallback when PawnIO is absent).

## Data location

Everything the app writes lives in a hidden `.open-monitoring` folder next to
the exe (`config.Dir()`): `config.json`, `sessions.db`, unpacked helpers under
`bin/`, and the WebView2 cache under `webview/`. Portable and installed builds
follow the same rule; files from the pre-portable `%AppData%\OpenMonitoring`
home are migrated in once and the old helper cache is removed. The manifest is
`requireAdministrator`, which is what makes Program Files writable.

## Gotchas that will bite again

- **PawnIO is mandatory**: when the sensor helper reports `pawnIo: false`, the
  frontend blocks the whole app behind `PawnIoGate.svelte` until the driver is
  installed (gate condition: `sensorsOk && !pawnIo` — unknown state keeps the
  dashboard up so dev builds without helpers still run). LHM checks for the
  driver ONCE at type init — after installing, the sensor helper must be
  restarted (`Collector.RestartSensors()`), or it never sees it.
- **FPS runs on its own clock**: `Collector.runFPS` emits an `fps` event every
  100 ms, separate from the `sample` stream — the HUD readout and its graphs
  read that, recording still reads samples. PresentMon's CSV reaches the pipe
  in buffered bursts, not frame by frame, so every FPS window is measured by
  walking back over frame *times* (`recentRate`/`recentPeak`), never by arrival
  timestamps; `at` is only good for deciding a process went quiet
  (`staleAfter`). Frames over `maxFrameMs` are alt-tab gaps, not frames.
- **Hotkeys** (`internal/hotkey`): `RegisterHotKey` binds to the *calling
  thread*, and only that thread's queue gets WM_HOTKEY — hence the locked OS
  thread and the `GetMessage` loop. Combinations are config strings; a bare key
  parses fine but is claimed machine-wide, so the defaults carry Ctrl+Alt.
  There is no Win32 call to amend a registration, so changing one means
  `Manager.Stop()` then `Register()` again (`App.reregisterHotkeys`, driven from
  `SaveConfig`). `Register` returns one error slot per binding, nil where the
  shortcut is live — Settings reads that through `GetHotkeyStatus`, because a
  combination another app already owns fails silently and looks exactly like a
  broken feature. The capture field only offers what `parse` supports (letters,
  digits, F1–F24).
- **Wails bindings**: the generator does not descend into embedded structs to
  find named types — declare fields directly on the bound struct (see
  `StaticInfo` / `ApplyBridgeInfo`). Embedded structs of primitives are fine.
- **WMI**: all queries go through one OS-thread-locked worker
  (`wmi_windows.go`); disk health is cached off the request path. Write WMI
  SELECT strings manually — `wmi.CreateQuery` derives the class from the Go
  struct name.
- **HUD**: Wails always-on-top is one-shot; `window_windows.go` re-asserts
  HWND_TOPMOST every 700 ms. `windows.NewCallback` is created once at package
  scope — the runtime never frees callbacks. Exclusive fullscreen is
  unreachable without a D3D hook (the UI warns).
- **Frontend buffers**: `chartDefs.js: newBuffers()/appendSample()` is the one
  sample→series mapping; the live ring (`state.svelte.js: buf`) and recorded
  sessions share it. Buffers are deliberately non-reactive; `live.tick` is the
  redraw signal.
- **Stress test**: no cgo is available, so nothing here can link a vendor SDK.
  CPU vector throughput comes from hand-written `cpu_amd64.s` (AVX-512 and AVX2
  FMA; `cpu_other.go` keeps non-amd64 building), and the GPU is driven through
  `OpenCL.dll` — the ICD loader every vendor drops in System32 — called by
  `syscall` in `opencl_windows.go`, with the kernels compiled by the driver at
  run time. Keep each GPU dispatch under ~250 ms: Windows resets a display
  driver that has not returned in two seconds. Disk uses
  `FILE_FLAG_NO_BUFFERING`, so buffers, lengths *and* offsets must be
  4 KB-aligned (`alignedBuf`) — otherwise every I/O fails with EINVAL.
- The sensor helper takes a few seconds to first report — GPU name and PawnIO
  status are unknown at startup; `state.svelte.js` polls `GetStaticInfo` until
  `sensorsOk`.
- Build script: the `dotnet` on PATH may be runtime-only; `build.ps1` resolves
  the SDK itself (first candidate whose `--list-sdks` succeeds).
- Branding: the logo (abstract CPU + rising chart) lives in `assets/logo.svg`;
  `frontend/src/lib/Logo.svelte` mirrors it inline, and `build/appicon.png`
  is a GDI+ rasterization of the same geometry (wails regenerates
  `build/windows/icon.ico` from it when the .ico is absent). Change all three
  together.
- The About tab inlines the repo README via a Vite `?raw` import from outside
  the frontend root — `vite.config.js` has `server.fs.allow: ['..']` for it;
  links are routed to the system browser through `BrowserOpenURL`. It drops the
  Screenshots and Download sections with a regex that **must** allow `\r?\n`:
  the README is checked out with CRLF, so a `\n`-only pattern matches nothing
  and fails silently — which is how screenshots once rendered there as captions
  with no images. The version shown comes from `wails.json`, the same source as
  the exe's file properties and the installer pages.
- CI (`.github/workflows/release.yml`): every push to main/master runs
  `build.ps1 -Installer` on windows-latest and updates the rolling `latest`
  GitHub Release with the portable exe and the NSIS installer. The installer
  (`build/windows/installer/project.nsi`) has a components page: the app
  (read-only) + the PawnIO driver via winget, checked by default; uninstall
  leaves PawnIO in place.
