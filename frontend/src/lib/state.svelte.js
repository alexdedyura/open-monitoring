import {EventsOn} from '../../wailsjs/runtime/runtime.js'
import * as api from '../../wailsjs/go/app/App.js'
import {newBuffers, appendSample} from './chartDefs.js'

// Client-side history window: up to 30 min of samples feed the live charts.
export const CAP_SECONDS = 1800

// Ring buffers are plain (non-reactive) arrays read directly by uPlot;
// `live.tick` is the reactive signal that a new sample arrived.
export const buf = newBuffers()

export const live = $state({
  ready: false, // backend handshake done (splash gate)
  tick: 0,
  sample: null,
  info: null,
  cfg: null,
  rec: {active: false},
  view: 'monitor', // monitor | sessions | settings
  hud: false,
  range: 300, // seconds shown in live charts
})

function pushSample(s) {
  appendSample(buf, s)
  if (buf.t.length > CAP_SECONDS) {
    for (const k in buf) buf[k].shift()
  }
}

let started = false

export async function init() {
  if (started) return
  started = true
  const [cfg, info, rec, hist] = await Promise.all([
    api.GetConfig(),
    api.GetStaticInfo(),
    api.GetRecStatus(),
    api.GetHistory(CAP_SECONDS),
  ])
  live.cfg = cfg
  live.info = info
  live.rec = rec
  for (const s of hist ?? []) pushSample(s)
  if (hist?.length) {
    live.sample = hist[hist.length - 1]
    live.tick++
  }
  EventsOn('sample', (s) => {
    pushSample(s)
    live.sample = s
    live.tick++
  })
  EventsOn('recording', (r) => {
    live.rec = r
  })
  live.ready = true
  pollInfoUntilSensorsReady()
}

// The sensor helper takes a few seconds to start and produce its first
// reading, and until it does the backend cannot know the GPU model or whether
// the PawnIO driver is present. Without this the HUD would keep a generic
// "GPU" header and Settings would claim the driver is missing on machines
// where it is installed. Give up quietly — a helper that never reports is a
// state the UI already renders correctly.
async function pollInfoUntilSensorsReady() {
  for (let attempt = 0; attempt < 10 && !live.info?.sensorsOk; attempt++) {
    await new Promise((resolve) => setTimeout(resolve, 2000))
    try {
      live.info = await api.GetStaticInfo()
    } catch {
      return // backend went away; nothing left to refresh
    }
  }
}

// refreshInfo re-reads the machine description on demand, for views that show
// source status the user may have just changed outside the app.
export async function refreshInfo() {
  live.info = await api.GetStaticInfo()
}

// installPawnIO runs the driver install and waits for the result to become
// visible. It is offered from two places — the startup notice and Settings —
// so the flow lives here rather than in either component.
//
// The wait is necessary: the backend relaunches the sensor helper, and the
// driver only shows as present once the replacement has produced a reading.
export async function installPawnIO() {
  const outcome = await api.InstallPawnIO()

  if (outcome.started) {
    for (let attempt = 0; attempt < 15 && !live.info?.pawnIo; attempt++) {
      await new Promise((resolve) => setTimeout(resolve, 1500))
      try {
        await refreshInfo()
      } catch {
        break
      }
    }
  }
  return {message: outcome.message, ok: outcome.started || outcome.openedSite}
}

export async function saveConfig(cfg) {
  await api.SaveConfig(cfg)
  live.cfg = cfg
}

export async function toggleTheme() {
  const cfg = $state.snapshot(live.cfg)
  if (!cfg) return
  cfg.theme = cfg.theme === 'light' ? 'dark' : 'light'
  await saveConfig(cfg)
}

export async function enterHud() {
  await api.SetHudMode(true)
  live.hud = true
}

export async function exitHud() {
  await api.SetHudMode(false)
  live.hud = false
}

export {api}
