// Shared uPlot option factory: recessive axes, mono type, crosshair with a
// live legend as the hover layer, optional drag-to-zoom (double-click resets).

const AXIS_INK = '#5f6873'
const GRID_INK = 'rgba(255,255,255,0.05)'
const MONO = '11px "Cascadia Mono", Consolas, monospace'

export function hhmmss(t) {
  return new Date(t * 1000).toTimeString().slice(0, 8)
}

function niceMax(max) {
  if (!isFinite(max) || max <= 0) return 1
  return max * 1.15
}

// series: [{label, color, digits?, fill?}] over buffer keys handled by caller
export function makeOpts({width = 600, height = 170, series, yMax = null, unit = '', zoom = false}) {
  return {
    width,
    height,
    padding: [8, 12, 0, 0],
    cursor: {
      y: false,
      drag: {x: zoom, y: false, setScale: zoom},
      points: {size: 5, width: 1},
    },
    select: {show: zoom},
    legend: {show: true, live: true},
    scales: {
      x: {time: true},
      y: {
        range: (u, min, max) => [0, yMax ?? niceMax(max)],
      },
    },
    axes: [
      {
        stroke: AXIS_INK,
        font: MONO,
        grid: {show: false},
        ticks: {show: false},
        space: 90,
        values: (u, ts) => ts.map(hhmmss),
      },
      {
        stroke: AXIS_INK,
        font: MONO,
        grid: {stroke: GRID_INK, width: 1},
        ticks: {show: false},
        size: 46,
      },
    ],
    series: [
      {value: (u, t) => (t == null ? '—' : hhmmss(t))},
      ...series.map((s) => ({
        label: s.label,
        stroke: s.color,
        width: 2,
        fill: s.fill ? s.color + '22' : undefined,
        points: {show: false},
        spanGaps: false,
        value: (u, v) => (v == null ? '—' : v.toFixed(s.digits ?? 0) + (unit ? ' ' + unit : '')),
      })),
    ],
  }
}
