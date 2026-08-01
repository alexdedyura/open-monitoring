import {palette, diskTotals} from './metricDefs.js'
import {t} from './i18n.svelte.js'

// The ten dashboard charts; the same definitions render live buffers and
// recorded sessions. `id` is the handle everything else holds a chart by — the
// {#each} keys and the pair the stress tab picks out — because `title` is
// display text, and display text is the first thing a translation rewrites;
// identifying a chart by the words on its header would empty that tab silently.
//
// `titleKey` / `labelKey` are the catalogue keys; `title` and `label` are kept
// beside them as the English source, only ever read as a fallback. This table
// is built at import, so nothing here may call t() — the callers translate the
// title at the render site and resolveSeries() translates the labels.
//
// `pkey` is resolved to a hex color for the active theme with resolveSeries().
// Within-chart series pairs are adjacent slots of the validated palette
// (CVD-checked in both themes).
export const CHARTS = [
  {
    id: 'cpu-load', title: 'CPU load', titleKey: 'm.chart.cpu-load', unit: '%', yMax: 100,
    series: [{key: 'cpu', label: 'CPU', labelKey: 'm.series.cpu', pkey: 'cpu', fill: true}],
  },
  {
    id: 'gpu-load', title: 'GPU load', titleKey: 'm.chart.gpu-load', unit: '%', yMax: 100,
    series: [{key: 'gpu', label: 'GPU', labelKey: 'm.series.gpu', pkey: 'gpu', fill: true}],
  },
  {
    id: 'temperature', title: 'Temperature', titleKey: 'm.chart.temperature', unit: '°C',
    series: [
      {key: 'cpuTemp', label: 'CPU', labelKey: 'm.series.cpu', pkey: 'cpu'},
      {key: 'gpuTemp', label: 'GPU', labelKey: 'm.series.gpu', pkey: 'gpu'},
    ],
  },
  {
    id: 'power-draw', title: 'Power draw', titleKey: 'm.chart.power-draw', unit: 'W',
    series: [
      {key: 'cpuPow', label: 'CPU', labelKey: 'm.series.cpu', pkey: 'cpu'},
      {key: 'gpuPow', label: 'GPU', labelKey: 'm.series.gpu', pkey: 'gpu'},
    ],
  },
  {
    id: 'ram-used', title: 'RAM used', titleKey: 'm.chart.ram-used', unit: 'GB', yMaxKey: 'ram',
    series: [{key: 'ram', label: 'Used', labelKey: 'm.series.used', pkey: 'ram', fill: true, digits: 1}],
  },
  {
    id: 'vram-used', title: 'VRAM used', titleKey: 'm.chart.vram-used', unit: 'GB', yMaxKey: 'vram',
    series: [{key: 'vram', label: 'Used', labelKey: 'm.series.used', pkey: 'vram', fill: true, digits: 1}],
  },
  {
    // Windows grows the page file under memory pressure, so its size is a
    // series of its own, not a fixed axis maximum.
    id: 'page-file', title: 'Page file (swap)', titleKey: 'm.chart.page-file', unit: 'GB',
    series: [
      {key: 'swapUsed', label: 'Used', labelKey: 'm.series.used', pkey: 'ram', fill: true, digits: 1},
      {key: 'swapTotal', label: 'Size', labelKey: 'm.series.size', pkey: 'vram', digits: 1},
    ],
  },
  {
    id: 'disk-io', title: 'Disk I/O', titleKey: 'm.chart.disk-io', unit: 'MB/s',
    series: [
      {key: 'diskR', label: 'Read', labelKey: 'm.series.read', pkey: 'diskR', digits: 1},
      {key: 'diskW', label: 'Write', labelKey: 'm.series.write', pkey: 'diskW', digits: 1},
    ],
  },
  {
    id: 'network', title: 'Network', titleKey: 'm.chart.network', unit: 'Mbps',
    series: [
      {key: 'netDown', label: 'Down', labelKey: 'm.series.down', pkey: 'netD', digits: 1},
      {key: 'netUp', label: 'Up', labelKey: 'm.series.up', pkey: 'netU', digits: 1},
    ],
  },
  {
    id: 'fps', title: 'FPS (foreground app)', titleKey: 'm.chart.fps', unit: 'fps',
    series: [{key: 'fps', label: 'FPS', labelKey: 'm.series.fps', pkey: 'fps'}],
  },
]

// Resolves a chart's series for display: palette color for the active theme,
// and the label in the active language. Both are display-time properties, which
// is why they are attached here and not stored on CHARTS — and why the callers
// re-key their chart grid on the language as well as the theme, since uPlot
// copies the legend labels at construction and never looks at them again.
export function resolveSeries(series, theme) {
  const pal = palette(theme)
  return series.map((s) => ({...s, color: pal[s.pkey], label: t(s.labelKey)}))
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
