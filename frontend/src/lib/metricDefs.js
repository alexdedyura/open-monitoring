// Registry of displayable metrics: one place that knows each metric's label,
// entity color and how to read it out of a Sample. Used by the HUD, the HUD
// settings editor and the stat tiles.

// Entity colors — validated categorical palette (dataviz skill), one set per
// theme. Chart pairs within one chart are adjacent validated slots.
export const PALETTES = {
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

const f0 = (v) => (v == null ? '—' : v.toFixed(0))
const f1 = (v) => (v == null ? '—' : v.toFixed(1))

export function diskTotals(s) {
  let dr = 0, dw = 0
  for (const d of s?.disks ?? []) {
    dr += d.readBps
    dw += d.writeBps
  }
  return [dr / 2 ** 20, dw / 2 ** 20] // MiB/s
}

// label: name in the settings editor; row: name of the HUD row inside its
// section; value(sample, info) -> display string.
export const METRICS = {
  gpu: {
    label: 'GPU load',
    row: 'Load',
    color: COLORS.gpu,
    value: (s) => (s.gpu ? f0(s.gpu.usage) + ' %' : '—'),
  },
  gpuTemp: {
    label: 'GPU temp',
    row: 'Temperature',
    color: COLORS.gpu,
    value: (s) => (s.gpu?.tempC ? f0(s.gpu.tempC) + ' °C' : '—'),
  },
  gpuClock: {
    label: 'GPU clock',
    row: 'Core clock',
    color: COLORS.gpu,
    value: (s) => (s.gpu?.coreMhz ? f0(s.gpu.coreMhz) + ' MHz' : '—'),
  },
  gpuMemClock: {
    label: 'GPU mem clock',
    row: 'Memory clock',
    color: COLORS.gpu,
    value: (s) => (s.gpu?.memMhz ? f0(s.gpu.memMhz) + ' MHz' : '—'),
  },
  gpuPow: {
    label: 'GPU power',
    row: 'Power',
    color: COLORS.gpu,
    value: (s) => (s.gpu?.powerW ? f1(s.gpu.powerW) + ' W' : '—'),
  },
  gpuFan: {
    label: 'GPU fan',
    row: 'Fan',
    color: COLORS.gpu,
    value: (s) => (s.gpu?.fanPercent ? f0(s.gpu.fanPercent) + ' %' : '—'),
  },
  vram: {
    label: 'VRAM',
    row: 'VRAM',
    color: COLORS.vram,
    value: (s) =>
      s.gpu?.memTotalMb
        ? `${f1(s.gpu.memUsedMb / 1024)} / ${f1(s.gpu.memTotalMb / 1024)} GB`
        : '—',
  },
  cpu: {
    label: 'CPU load',
    row: 'Load',
    color: COLORS.cpu,
    value: (s) => f0(s.cpu.usage) + ' %',
  },
  cpuTemp: {
    label: 'CPU temp',
    row: 'Temperature',
    color: COLORS.cpu,
    value: (s) => (s.cpu.tempC ? f0(s.cpu.tempC) + ' °C' : '—'),
  },
  cpuClock: {
    label: 'CPU clock',
    row: 'Core clock',
    color: COLORS.cpu,
    value: (s) => (s.cpu.clockMhz ? f0(s.cpu.clockMhz) + ' MHz' : '—'),
  },
  cpuPow: {
    label: 'CPU power',
    row: 'Power',
    color: COLORS.cpu,
    value: (s) => (s.cpu.powerW ? f1(s.cpu.powerW) + ' W' : '—'),
  },
  ram: {
    label: 'RAM used',
    row: 'Used',
    color: COLORS.ram,
    value: (s) => `${f1(s.mem.used / 2 ** 30)} / ${f1(s.mem.total / 2 ** 30)} GB`,
  },
  ramLoad: {
    label: 'RAM load',
    row: 'Load',
    color: COLORS.ram,
    value: (s) => f0(s.mem.usedPercent) + ' %',
  },
  ramSpeed: {
    label: 'RAM speed',
    row: 'Speed',
    color: COLORS.ram,
    value: (s, info) => (info?.ram?.speedMt ? f0(info.ram.speedMt) + ' MHz' : '—'),
  },
  diskRead: {
    label: 'Disk read',
    row: 'Read',
    color: COLORS.diskR,
    value: (s) => f1(diskTotals(s)[0]) + ' MB/s',
  },
  diskWrite: {
    label: 'Disk write',
    row: 'Write',
    color: COLORS.diskW,
    value: (s) => f1(diskTotals(s)[1]) + ' MB/s',
  },
  netDown: {
    label: 'Net download',
    row: 'Download',
    color: COLORS.netD,
    value: (s) => f1(s.net.downBps * 8 / 1e6) + ' Mbps',
  },
  netUp: {
    label: 'Net upload',
    row: 'Upload',
    color: COLORS.netU,
    value: (s) => f1(s.net.upBps * 8 / 1e6) + ' Mbps',
  },
  fps: {
    label: 'FPS',
    row: 'In-time',
    color: COLORS.fps,
    value: (s) => (s.fps ? f0(s.fps.cur) : '—'),
  },
  fpsAvg: {
    label: 'FPS average',
    row: 'Average',
    color: COLORS.fps,
    value: (s) => (s.fps ? f0(s.fps.avg) : '—'),
  },
  fpsLow1: {
    label: 'FPS 1% low',
    row: '1% low',
    color: COLORS.fps,
    value: (s) => (s.fps?.low1 ? f0(s.fps.low1) : '—'),
  },
  fpsLow01: {
    label: 'FPS 0.1% low',
    row: '0.1% low',
    color: COLORS.fps,
    value: (s) => (s.fps?.low01 ? f0(s.fps.low01) : '—'),
  },
  // Rendered as sparklines rather than value rows; see Hud.svelte.
  fpsGraph: {
    label: 'FPS graph',
    row: 'FPS graph',
    color: COLORS.fps,
    graph: {key: 'fps', unit: ' fps', digits: 0},
  },
  frameGraph: {
    label: 'Frame time graph',
    row: 'Frame time graph',
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

// HUD sections in on-screen order; header comes from live hardware names.
export const HUD_GROUPS = [
  {id: 'gpu', colorKey: 'gpu', header: (info) => cleanModel(info?.gpuName) || 'GPU', keys: ['gpu', 'gpuTemp', 'gpuClock', 'gpuMemClock', 'gpuPow', 'gpuFan', 'vram']},
  {id: 'cpu', colorKey: 'cpu', header: (info) => cleanModel(info?.cpuModel) || 'CPU', keys: ['cpu', 'cpuTemp', 'cpuClock', 'cpuPow']},
  {id: 'ram', colorKey: 'ram', header: () => 'Memory', keys: ['ram', 'ramLoad', 'ramSpeed']},
  {id: 'disk', colorKey: 'diskR', header: () => 'Storage', keys: ['diskRead', 'diskWrite']},
  {id: 'net', colorKey: 'netU', header: () => 'Network', keys: ['netDown', 'netUp']},
  {id: 'fps', colorKey: 'fps', header: () => 'FPS', keys: ['fps', 'fpsAvg', 'fpsLow1', 'fpsLow01', 'frameGraph', 'fpsGraph']},
]
