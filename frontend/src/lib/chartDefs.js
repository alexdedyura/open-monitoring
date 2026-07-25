import {palette, diskTotals} from './metricDefs.js'

// The eight dashboard charts; the same definitions render live buffers and
// recorded sessions. `pkey` is resolved to a hex color for the active theme
// with resolveSeries(). Within-chart series pairs are adjacent slots of the
// validated palette (CVD-checked in both themes).
export const CHARTS = [
  {title: 'CPU load', unit: '%', yMax: 100, series: [{key: 'cpu', label: 'CPU', pkey: 'cpu', fill: true}]},
  {title: 'GPU load', unit: '%', yMax: 100, series: [{key: 'gpu', label: 'GPU', pkey: 'gpu', fill: true}]},
  {
    title: 'Temperature', unit: '°C',
    series: [
      {key: 'cpuTemp', label: 'CPU', pkey: 'cpu'},
      {key: 'gpuTemp', label: 'GPU', pkey: 'gpu'},
    ],
  },
  {
    title: 'Power draw', unit: 'W',
    series: [
      {key: 'cpuPow', label: 'CPU', pkey: 'cpu'},
      {key: 'gpuPow', label: 'GPU', pkey: 'gpu'},
    ],
  },
  {title: 'RAM used', unit: 'GB', yMaxKey: 'ram', series: [{key: 'ram', label: 'Used', pkey: 'ram', fill: true, digits: 1}]},
  {title: 'VRAM used', unit: 'GB', yMaxKey: 'vram', series: [{key: 'vram', label: 'Used', pkey: 'vram', fill: true, digits: 1}]},
  {
    // Windows grows the page file under memory pressure, so its size is a
    // series of its own, not a fixed axis maximum.
    title: 'Page file (swap)', unit: 'GB',
    series: [
      {key: 'swapUsed', label: 'Used', pkey: 'ram', fill: true, digits: 1},
      {key: 'swapTotal', label: 'Size', pkey: 'vram', digits: 1},
    ],
  },
  {
    title: 'Disk I/O', unit: 'MB/s',
    series: [
      {key: 'diskR', label: 'Read', pkey: 'diskR', digits: 1},
      {key: 'diskW', label: 'Write', pkey: 'diskW', digits: 1},
    ],
  },
  {
    title: 'Network', unit: 'Mbps',
    series: [
      {key: 'netDown', label: 'Down', pkey: 'netD', digits: 1},
      {key: 'netUp', label: 'Up', pkey: 'netU', digits: 1},
    ],
  },
  {title: 'FPS (foreground app)', unit: 'fps', series: [{key: 'fps', label: 'FPS', pkey: 'fps'}]},
]

export function resolveSeries(series, theme) {
  const pal = palette(theme)
  return series.map((s) => ({...s, color: pal[s.pkey]}))
}

// One buffer set holds a series array per chartable key. The live ring buffers
// and the recorded-session charts share this shape — appendSample is the single
// sample→series mapping. (The HUD's frame-time graph is not here: it runs off
// the faster frame-rate event, in `fpsBuf`.)
export function newBuffers() {
  return {
    t: [],
    cpu: [], cpuTemp: [], cpuPow: [], ram: [], swapUsed: [], swapTotal: [],
    gpu: [], gpuTemp: [], gpuPow: [], vram: [],
    diskR: [], diskW: [], netUp: [], netDown: [],
    fps: [],
  }
}

export function appendSample(b, s) {
  const g = s.gpu ?? null
  const [dr, dw] = diskTotals(s)
  b.t.push(s.t / 1000)
  b.cpu.push(s.cpu.usage)
  b.cpuTemp.push(s.cpu.tempC || null)
  b.cpuPow.push(s.cpu.powerW || null)
  b.ram.push(s.mem.used / 2 ** 30)
  b.swapUsed.push(s.mem.swapTotal ? s.mem.swapUsed / 2 ** 30 : null)
  b.swapTotal.push(s.mem.swapTotal ? s.mem.swapTotal / 2 ** 30 : null)
  b.gpu.push(g ? g.usage : null)
  b.gpuTemp.push(g && g.tempC ? g.tempC : null)
  b.gpuPow.push(g && g.powerW ? g.powerW : null)
  b.vram.push(g && g.memUsedMb ? g.memUsedMb / 1024 : null)
  b.diskR.push(dr)
  b.diskW.push(dw)
  b.netUp.push(s.net.upBps * 8 / 1e6)
  b.netDown.push(s.net.downBps * 8 / 1e6)
  b.fps.push(s.fps?.cur ?? null)
}

// Turn recorded samples into the same buffer shape the live charts use.
export function buildBuffers(samples) {
  const b = newBuffers()
  for (const s of samples) appendSample(b, s)
  return b
}
