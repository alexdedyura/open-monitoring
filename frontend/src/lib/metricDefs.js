// Registry of displayable metrics: one place that knows each metric's label,
// entity color and how to read it out of a Sample. Used by the HUD, the HUD
// settings editor and the stat tiles.

export const COLORS = {
  cpu: '#3987e5',
  gpu: '#d95926',
  ram: '#199e70',
  vram: '#c98500',
  diskR: '#9085e9',
  diskW: '#e66767',
  netD: '#008300',
  netU: '#d55181',
}

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

// value(s) -> display string; pct(s) -> 0..100 for the capacity bar (optional)
export const METRICS = {
  cpu: {
    label: 'CPU',
    color: COLORS.cpu,
    value: (s) => f0(s.cpu.usage) + '%',
    pct: (s) => s.cpu.usage,
  },
  cpuTemp: {
    label: 'CPU °C',
    color: COLORS.cpu,
    value: (s) => (s.cpu.tempC ? f0(s.cpu.tempC) + '°' : '—'),
  },
  cpuPow: {
    label: 'CPU W',
    color: COLORS.cpu,
    value: (s) => (s.cpu.powerW ? f0(s.cpu.powerW) + ' W' : '—'),
  },
  cpuClock: {
    label: 'CPU MHz',
    color: COLORS.cpu,
    value: (s) => (s.cpu.clockMhz ? f0(s.cpu.clockMhz) : '—'),
  },
  ram: {
    label: 'RAM',
    color: COLORS.ram,
    value: (s) => f1(s.mem.used / 2 ** 30) + ' GB',
    pct: (s) => s.mem.usedPercent,
  },
  gpu: {
    label: 'GPU',
    color: COLORS.gpu,
    value: (s) => (s.gpu ? f0(s.gpu.usage) + '%' : '—'),
    pct: (s) => s.gpu?.usage ?? 0,
  },
  gpuTemp: {
    label: 'GPU °C',
    color: COLORS.gpu,
    value: (s) => (s.gpu?.tempC ? f0(s.gpu.tempC) + '°' : '—'),
  },
  gpuPow: {
    label: 'GPU W',
    color: COLORS.gpu,
    value: (s) => (s.gpu?.powerW ? f0(s.gpu.powerW) + ' W' : '—'),
  },
  gpuClock: {
    label: 'GPU MHz',
    color: COLORS.gpu,
    value: (s) => (s.gpu?.coreMhz ? f0(s.gpu.coreMhz) : '—'),
  },
  gpuFan: {
    label: 'GPU Fan',
    color: COLORS.gpu,
    value: (s) => (s.gpu?.fanPercent ? f0(s.gpu.fanPercent) + '%' : '—'),
  },
  vram: {
    label: 'VRAM',
    color: COLORS.vram,
    value: (s) => (s.gpu?.memUsedMb ? f1(s.gpu.memUsedMb / 1024) + ' GB' : '—'),
    pct: (s) => (s.gpu?.memTotalMb ? (s.gpu.memUsedMb / s.gpu.memTotalMb) * 100 : 0),
  },
  disk: {
    label: 'DISK',
    color: COLORS.diskR,
    value: (s) => {
      const [dr, dw] = diskTotals(s)
      return `R ${f0(dr)} W ${f0(dw)}`
    },
  },
  net: {
    label: 'NET',
    color: COLORS.netU,
    value: (s) => `↓${f1(s.net.downBps * 8 / 1e6)} ↑${f1(s.net.upBps * 8 / 1e6)}`,
  },
}
