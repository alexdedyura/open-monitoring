import {EventsOn} from '../../wailsjs/runtime/runtime.js'
import * as api from '../../wailsjs/go/main/App.js'

// Client-side history window: up to 30 min of samples feed the live charts.
export const CAP_SECONDS = 1800

// Ring buffers are plain (non-reactive) arrays read directly by uPlot;
// `live.tick` is the reactive signal that a new sample arrived.
export const buf = {
  t: [],
  cpu: [], cpuTemp: [], cpuPow: [], cpuClock: [],
  ram: [],
  gpu: [], gpuTemp: [], gpuPow: [], gpuClock: [], gpuFan: [],
  vram: [],
  diskR: [], diskW: [],
  netUp: [], netDown: [],
}

export const live = $state({
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
  const g = s.gpu ?? null
  buf.t.push(s.t / 1000)
  buf.cpu.push(s.cpu.usage)
  buf.cpuTemp.push(s.cpu.tempC || null)
  buf.cpuPow.push(s.cpu.powerW || null)
  buf.cpuClock.push(s.cpu.clockMhz || null)
  buf.ram.push(s.mem.used / 2 ** 30)
  buf.gpu.push(g ? g.usage : null)
  buf.gpuTemp.push(g && g.tempC ? g.tempC : null)
  buf.gpuPow.push(g && g.powerW ? g.powerW : null)
  buf.gpuClock.push(g && g.coreMhz ? g.coreMhz : null)
  buf.gpuFan.push(g && g.fanPercent ? g.fanPercent : null)
  buf.vram.push(g && g.memUsedMb ? g.memUsedMb / 1024 : null)
  let dr = 0, dw = 0
  for (const d of s.disks ?? []) {
    dr += d.readBps
    dw += d.writeBps
  }
  buf.diskR.push(dr / 2 ** 20)
  buf.diskW.push(dw / 2 ** 20)
  buf.netUp.push(s.net.upBps * 8 / 1e6)
  buf.netDown.push(s.net.downBps * 8 / 1e6)
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
}

export async function saveConfig(cfg) {
  await api.SaveConfig(cfg)
  live.cfg = cfg
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
