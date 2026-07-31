// Shared uPlot option factory: recessive axes, mono type, crosshair with a
// live legend as the hover layer, optional drag-to-zoom (double-click resets).

const CHROME = {
  dark: {axis: '#5f6873', grid: 'rgba(255,255,255,0.05)'},
  light: {axis: '#8a94a0', grid: 'rgba(15,23,42,0.07)'},
}

const MONO = '11px "Cascadia Mono", Consolas, monospace'

function hhmmss(t) {
  return new Date(t * 1000).toTimeString().slice(0, 8)
}

// Round the top of the axis up to a round number. A bare max * 1.15 re-ranges
// on every sample, so the gridlines and their labels twitch continuously under
// a line that has barely moved — the axis ends up looking noisier than the data.
// Snapping to a step means it only changes when the data genuinely outgrows it.
const STEPS = [1, 1.5, 2, 2.5, 3, 4, 5, 7.5, 10]

function niceMax(max) {
  if (!isFinite(max) || max <= 0) return 1
  const target = max * 1.15
  const mag = 10 ** Math.floor(Math.log10(target))
  for (const step of STEPS) {
    if (step * mag >= target) return step * mag
  }
  return 10 * mag
}

// series: [{label, color, digits?, fill?}] over buffer keys handled by caller
//
// `smooth` is for the live charts, which scroll continuously: uPlot snaps
// everything it draws to whole pixels by default, and a line creeping sideways
// at a fraction of a pixel per frame then advances in visible one-pixel
// lurches. Static charts keep the snapping — nothing moves, and crisp is
// better than smeared.
export function makeOpts({width = 600, height = 170, series, yMax = null, unit = '', zoom = false, theme = 'dark', smooth = false}) {
  const ink = CHROME[theme] ?? CHROME.dark
  return {
    width,
    height,
    padding: [8, 12, 0, 0],
    pxAlign: smooth ? 0 : 1, // series inherit this
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
        stroke: ink.axis,
        font: MONO,
        grid: {show: false},
        ticks: {show: false},
        space: 90,
        values: (u, ts) => ts.map(hhmmss),
      },
      {
        stroke: ink.axis,
        font: MONO,
        grid: {stroke: ink.grid, width: 1},
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
