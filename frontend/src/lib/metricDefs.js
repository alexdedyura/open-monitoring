// Registry of displayable metrics: one place that knows each metric's name,
// entity color and how to read it out of a Sample. Used by the HUD, the HUD
// settings editor and the stat tiles.
//
// This module is evaluated once, at import — so it holds catalogue KEYS, never
// translated text. Calling t() here would freeze every label in whatever
// language was active at startup (see i18n.svelte.js). The one exception is
// inside the HUD_GROUPS header functions at the bottom, which are *called* from
// a template and therefore run inside a tracked context.

import {t, fmtNum} from './i18n.svelte.js'

// Entity colors — validated categorical palette (dataviz skill), one set per
// theme. Chart pairs within one chart are adjacent validated slots.
const PALETTES = {
  dark: {
    cpu: '#3987e5',
    gpu: '#d95926',
    ram: '#199e70',
    vram: '#c98500',
    diskR: '#9085e9',
    diskW: '#e66767',
    netD: '#008300',
    netU: '#d55181',
    fps: '#e66767',
  },
  light: {
    cpu: '#2a78d6',
    gpu: '#eb6834',
    ram: '#1baf7a',
    vram: '#eda100',
    diskR: '#4a3aa7',
    diskW: '#e34948',
    netD: '#008300',
    netU: '#e87ba4',
    fps: '#e34948',
  },
}

export function palette(theme) {
  return PALETTES[theme] ?? PALETTES.dark
}

// The HUD overlay is always dark (it sits over games), so METRICS use the
// dark set regardless of app theme.
export const COLORS = PALETTES.dark

// Shared "value or em dash" formatters, used wherever a metric is printed.
// They go through fmtNum so the decimal mark follows the active language — a
// panel full of English decimal points reads as untranslated whatever the words
// around it say. The unit symbols concatenated onto these stay Latin/SI.
export const f0 = (v) => fmtNum(v, 0)
export const f1 = (v) => fmtNum(v, 1)

export function diskTotals(s) {
  let dr = 0, dw = 0
  for (const d of s?.disks ?? []) {
    dr += d.readBps
    dw += d.writeBps
  }
  return [dr / 2 ** 20, dw / 2 ** 20] // MiB/s
}

// rowKey: catalogue key for the metric's name inside its section — the HUD
// prints it and the settings editor labels the toggle with the same key, so a
// row reads the same in both places. It is a KEY, not text: this table is built
// once at import, so a t() call here would freeze in the startup language;
// Hud.svelte and Settings.svelte translate it at the point of use.
// value(sample, info) -> display string.
export const METRICS = {
  gpu: {
    rowKey: 'm.row.load',
    color: COLORS.gpu,
    value: (s) => (s.gpu ? f0(s.gpu.usage) + ' %' : '—'),
  },
  gpuTemp: {
    rowKey: 'm.row.temp',
    color: COLORS.gpu,
    value: (s) => (s.gpu?.tempC ? f0(s.gpu.tempC) + ' °C' : '—'),
  },
  gpuHotspot: {
    rowKey: 'm.row.hotspot',
    color: COLORS.gpu,
    value: (s) => (s.gpu?.hotspotC ? f0(s.gpu.hotspotC) + ' °C' : '—'),
  },
  gpuClock: {
    rowKey: 'm.row.clock',
    color: COLORS.gpu,
    value: (s) => (s.gpu?.coreMhz ? f0(s.gpu.coreMhz) + ' MHz' : '—'),
  },
  gpuMemClock: {
    rowKey: 'm.row.memclock',
    color: COLORS.gpu,
    value: (s) => (s.gpu?.memMhz ? f0(s.gpu.memMhz) + ' MHz' : '—'),
  },
  gpuPow: {
    rowKey: 'm.row.power',
    color: COLORS.gpu,
    value: (s) => (s.gpu?.powerW ? f1(s.gpu.powerW) + ' W' : '—'),
  },
  gpuFan: {
    rowKey: 'm.row.fan',
    color: COLORS.gpu,
    value: (s) => (s.gpu?.fanPercent ? f0(s.gpu.fanPercent) + ' %' : '—'),
  },
  vram: {
    rowKey: 'm.row.vram',
    color: COLORS.vram,
    value: (s) =>
      s.gpu?.memTotalMb
        ? `${f1(s.gpu.memUsedMb / 1024)} / ${f1(s.gpu.memTotalMb / 1024)} GB`
        : '—',
  },
  cpu: {
    rowKey: 'm.row.load',
    color: COLORS.cpu,
    value: (s) => f0(s.cpu.usage) + ' %',
  },
  cpuTemp: {
    rowKey: 'm.row.temp',
    color: COLORS.cpu,
    value: (s) => (s.cpu.tempC ? f0(s.cpu.tempC) + ' °C' : '—'),
  },
  cpuClock: {
    rowKey: 'm.row.clock',
    color: COLORS.cpu,
    value: (s) => (s.cpu.clockMhz ? f0(s.cpu.clockMhz) + ' MHz' : '—'),
  },
  cpuPow: {
    rowKey: 'm.row.power',
    color: COLORS.cpu,
    value: (s) => (s.cpu.powerW ? f1(s.cpu.powerW) + ' W' : '—'),
  },
  ram: {
    rowKey: 'm.row.used',
    color: COLORS.ram,
    value: (s) => `${f1(s.mem.used / 2 ** 30)} / ${f1(s.mem.total / 2 ** 30)} GB`,
  },
  ramLoad: {
    rowKey: 'm.row.load',
    color: COLORS.ram,
    value: (s) => f0(s.mem.usedPercent) + ' %',
  },
  ramSpeed: {
    rowKey: 'm.row.ramspeed',
    color: COLORS.ram,
    value: (s, info) => (info?.ram?.speedMt ? f0(info.ram.speedMt) + ' MHz' : '—'),
  },
  diskRead: {
    rowKey: 'm.row.read',
    color: COLORS.diskR,
    value: (s) => f1(diskTotals(s)[0]) + ' MB/s',
  },
  diskWrite: {
    rowKey: 'm.row.write',
    color: COLORS.diskW,
    value: (s) => f1(diskTotals(s)[1]) + ' MB/s',
  },
  netDown: {
    rowKey: 'm.row.down',
    color: COLORS.netD,
    value: (s) => f1(s.net.downBps * 8 / 1e6) + ' Mbps',
  },
  netUp: {
    rowKey: 'm.row.up',
    color: COLORS.netU,
    value: (s) => f1(s.net.upBps * 8 / 1e6) + ' Mbps',
  },
  fps: {
    rowKey: 'm.row.fps.cur',
    color: COLORS.fps,
    value: (s) => (s.fps ? f0(s.fps.cur) : '—'),
  },
  fpsAvg: {
    rowKey: 'm.row.fps.avg',
    color: COLORS.fps,
    value: (s) => (s.fps ? f0(s.fps.avg) : '—'),
  },
  fpsLow1: {
    rowKey: 'm.row.fps.low1',
    color: COLORS.fps,
    value: (s) => (s.fps?.low1 ? f0(s.fps.low1) : '—'),
  },
  fpsLow01: {
    rowKey: 'm.row.fps.low01',
    color: COLORS.fps,
    value: (s) => (s.fps?.low01 ? f0(s.fps.low01) : '—'),
  },
  // Rendered as sparklines rather than value rows; see Hud.svelte.
  fpsGraph: {
    rowKey: 'm.row.fps.graph',
    color: COLORS.fps,
    graph: {key: 'fps', unit: ' fps', digits: 0},
  },
  frameGraph: {
    rowKey: 'm.row.frametime.graph',
    color: COLORS.vram,
    graph: {key: 'frameMs', unit: ' ms', digits: 1, invert: true},
  },
}

export function cleanModel(name) {
  return (name ?? '')
    .replaceAll('(R)', '')
    .replaceAll('(TM)', '')
    .replace(/\s+CPU\s+@.*$/, '')
    .replace(/\s{2,}/g, ' ')
    .trim()
}

// HUD sections in on-screen order; header comes from live hardware names, and
// falls back to the subsystem's own name when the helper has not reported one.
//
// These t() calls are safe where the ones above would not be: `header` is a
// function, so nothing is translated at import — the call happens when a
// template invokes it (Hud.svelte, Settings.svelte), inside Svelte's tracked
// read. Keep it a function for exactly that reason.
export const HUD_GROUPS = [
  {id: 'gpu', colorKey: 'gpu', header: (info) => cleanModel(info?.gpuName) || t('m.group.gpu'), keys: ['gpu', 'gpuTemp', 'gpuHotspot', 'gpuClock', 'gpuMemClock', 'gpuPow', 'gpuFan', 'vram']},
  {id: 'cpu', colorKey: 'cpu', header: (info) => cleanModel(info?.cpuModel) || t('m.group.cpu'), keys: ['cpu', 'cpuTemp', 'cpuClock', 'cpuPow']},
  {id: 'ram', colorKey: 'ram', header: () => t('m.group.ram'), keys: ['ram', 'ramLoad', 'ramSpeed']},
  {id: 'disk', colorKey: 'diskR', header: () => t('m.group.storage'), keys: ['diskRead', 'diskWrite']},
  {id: 'net', colorKey: 'netU', header: () => t('m.group.net'), keys: ['netDown', 'netUp']},
  {id: 'fps', colorKey: 'fps', header: () => t('m.group.fps'), keys: ['fps', 'fpsAvg', 'fpsLow1', 'fpsLow01', 'frameGraph', 'fpsGraph']},
]
