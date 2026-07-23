import {COLORS} from './metricDefs.js'

// The eight dashboard charts; the same definitions render live buffers and
// recorded sessions. Within-chart series pairs are adjacent slots of the
// validated palette (CVD-checked).
export const CHARTS = [
  {title: 'CPU load', unit: '%', yMax: 100, series: [{key: 'cpu', label: 'CPU', color: COLORS.cpu, fill: true}]},
  {title: 'GPU load', unit: '%', yMax: 100, series: [{key: 'gpu', label: 'GPU', color: COLORS.gpu, fill: true}]},
  {
    title: 'Temperature', unit: '°C',
    series: [
      {key: 'cpuTemp', label: 'CPU', color: COLORS.cpu},
      {key: 'gpuTemp', label: 'GPU', color: COLORS.gpu},
    ],
  },
  {
    title: 'Power draw', unit: 'W',
    series: [
      {key: 'cpuPow', label: 'CPU', color: COLORS.cpu},
      {key: 'gpuPow', label: 'GPU', color: COLORS.gpu},
    ],
  },
  {title: 'RAM used', unit: 'GB', yMaxKey: 'ram', series: [{key: 'ram', label: 'Used', color: COLORS.ram, fill: true, digits: 1}]},
  {title: 'VRAM used', unit: 'GB', yMaxKey: 'vram', series: [{key: 'vram', label: 'Used', color: COLORS.vram, fill: true, digits: 1}]},
  {
    title: 'Disk I/O', unit: 'MB/s',
    series: [
      {key: 'diskR', label: 'Read', color: COLORS.diskR, digits: 1},
      {key: 'diskW', label: 'Write', color: COLORS.diskW, digits: 1},
    ],
  },
  {
    title: 'Network', unit: 'Mbps',
    series: [
      {key: 'netDown', label: 'Down', color: COLORS.netD, digits: 1},
      {key: 'netUp', label: 'Up', color: COLORS.netU, digits: 1},
    ],
  },
]

// Turn recorded samples into the same buffer shape the live charts use.
export function buildBuffers(samples) {
  const b = {
    t: [], cpu: [], cpuTemp: [], cpuPow: [], cpuClock: [], ram: [],
    gpu: [], gpuTemp: [], gpuPow: [], gpuClock: [], gpuFan: [], vram: [],
    diskR: [], diskW: [], netUp: [], netDown: [],
  }
  for (const s of samples) {
    const g = s.gpu ?? null
    b.t.push(s.t / 1000)
    b.cpu.push(s.cpu.usage)
    b.cpuTemp.push(s.cpu.tempC || null)
    b.cpuPow.push(s.cpu.powerW || null)
    b.cpuClock.push(s.cpu.clockMhz || null)
    b.ram.push(s.mem.used / 2 ** 30)
    b.gpu.push(g ? g.usage : null)
    b.gpuTemp.push(g && g.tempC ? g.tempC : null)
    b.gpuPow.push(g && g.powerW ? g.powerW : null)
    b.gpuClock.push(g && g.coreMhz ? g.coreMhz : null)
    b.gpuFan.push(g && g.fanPercent ? g.fanPercent : null)
    b.vram.push(g && g.memUsedMb ? g.memUsedMb / 1024 : null)
    let dr = 0, dw = 0
    for (const d of s.disks ?? []) {
      dr += d.readBps
      dw += d.writeBps
    }
    b.diskR.push(dr / 2 ** 20)
    b.diskW.push(dw / 2 ** 20)
    b.netUp.push(s.net.upBps * 8 / 1e6)
    b.netDown.push(s.net.downBps * 8 / 1e6)
  }
  return b
}
