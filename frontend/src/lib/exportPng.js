// Renders a recorded session as one PNG: the same chart definitions the app
// draws, composed on a canvas with the session's name and metadata. Rendering
// happens here because the charts only exist in the frontend; the Go side
// just asks where to save and writes the bytes.
import uPlot from 'uplot'
import * as api from '../../wailsjs/go/app/App.js'
import {CHARTS, resolveSeries, buildBuffers} from './chartDefs.js'
import {COLORS} from './metricDefs.js'
import {makeOpts} from './uplotOpts.js'
// Nothing here is reactive — the image is painted once, when the user asks for
// it — so calling t() imperatively is exactly right: it reads whatever language
// is active at that moment and bakes it into the PNG.
import {t, fmtNum, lang} from './i18n.svelte.js'

const THEMES = {
  dark: {page: '#0e1013', card: '#15181d', line: 'rgba(255,255,255,0.07)', ink: '#eef1f4', ink2: '#9aa3ad', mut: '#5f6873'},
  light: {page: '#eef1f5', card: '#ffffff', line: 'rgba(15,23,42,0.10)', ink: '#16191e', ink2: '#4b5563', mut: '#8a94a0'},
}

const MONO = '"Cascadia Mono", Consolas, monospace'

// exportSessionPNG loads the session, renders the image and hands it to the
// backend, which shows the save dialog. Resolves to the saved path, or ''
// when the user cancelled.
export async function exportSessionPNG(session, theme = 'dark') {
  const samples = (await api.GetSessionSamples(session.id)) ?? []
  const png = await render(session, buildBuffers(samples), theme)
  return api.ExportSessionPNG(session.id, png)
}

async function render(session, bufs, theme) {
  // Named `chrome`, not `t`: `t` is the translator, and shadowing it here threw
  // on the first chart title of every export — silently, because the caller has
  // no catch and the button simply did nothing.
  const chrome = THEMES[theme] ?? THEMES.dark

  // One full-width chart per row, stacked — a long readable timeline each.
  const CHART_W = 1360
  const CHART_H = 240
  const PAD = 12 // tile padding
  const TITLE_H = 24
  const GAP = 12
  const MARGIN = 20
  const HEADER = 62

  const tileW = CHART_W + PAD * 2
  const tileH = CHART_H + TITLE_H + PAD * 2
  const W = MARGIN * 2 + tileW
  const H = MARGIN + HEADER + CHARTS.length * tileH + (CHARTS.length - 1) * GAP + MARGIN

  const dpr = Math.min(1.5, window.devicePixelRatio || 1)
  const canvas = document.createElement('canvas')
  canvas.width = W * dpr
  canvas.height = H * dpr
  const ctx = canvas.getContext('2d')
  ctx.scale(dpr, dpr)

  ctx.fillStyle = chrome.page
  ctx.fillRect(0, 0, W, H)

  drawHeader(ctx, session, chrome, MARGIN, W)

  // uPlot needs a live element; parked off-screen for the duration.
  const host = document.createElement('div')
  host.style.cssText = 'position:fixed;left:-10000px;top:0'
  document.body.appendChild(host)

  try {
    for (let i = 0; i < CHARTS.length; i++) {
      const c = CHARTS[i]
      const x = MARGIN
      const y = MARGIN + HEADER + i * (tileH + GAP)

      ctx.fillStyle = chrome.card
      ctx.strokeStyle = chrome.line
      ctx.beginPath()
      ctx.roundRect(x + 0.5, y + 0.5, tileW - 1, tileH - 1, 8)
      ctx.fill()
      ctx.stroke()

      ctx.fillStyle = chrome.mut
      ctx.font = `10px ${MONO}`
      ctx.textBaseline = 'alphabetic'
      // chartDefs holds display text until it holds a catalogue key; take
      // whichever it offers so the tile header never renders a raw key.
      ctx.fillText((c.titleKey ? t(c.titleKey) : c.title).toUpperCase(), x + PAD, y + PAD + 9)

      const series = resolveSeries(c.series, theme)

      // series legend, right-aligned in the title row
      let lx = x + tileW - PAD
      for (let s = series.length - 1; s >= 0; s--) {
        const label = series[s].labelKey ? t(series[s].labelKey) : series[s].label
        const w = ctx.measureText(label).width
        lx -= w
        ctx.fillStyle = chrome.ink2
        ctx.fillText(label, lx, y + PAD + 9)
        lx -= 12
        ctx.fillStyle = series[s].color
        ctx.fillRect(lx, y + PAD + 2, 8, 8)
        lx -= 14
      }

      const data = [bufs.t, ...c.series.map((sd) => bufs[sd.key])]
      const u = new uPlot(
        makeOpts({width: CHART_W, height: CHART_H, series, yMax: c.yMax ?? null, unit: c.unit, theme}),
        data,
        host,
      )
      // uPlot commits its drawing through the microtask queue (see its own
      // svg-image demo) — reading the canvas synchronously captures nothing.
      await new Promise((resolve) => queueMicrotask(resolve))
      ctx.drawImage(u.ctx.canvas, x + PAD, y + PAD + TITLE_H, CHART_W, CHART_H)
      u.destroy()
    }
  } finally {
    host.remove()
  }

  return canvas.toDataURL('image/png').split(',')[1]
}

function drawHeader(ctx, session, chrome, margin, width) {
  // the four-dot brand mark
  const dots = [COLORS.cpu, COLORS.gpu, COLORS.ram, COLORS.vram]
  dots.forEach((c, i) => {
    ctx.fillStyle = c
    ctx.beginPath()
    ctx.roundRect(margin + (i % 2) * 9, 14 + ((i / 2) | 0) * 9, 7, 7, 1.5)
    ctx.fill()
  })

  ctx.fillStyle = chrome.ink2
  ctx.font = `11px ${MONO}`
  ctx.fillText('OPEN MONITORING', margin + 26, 25)

  ctx.fillStyle = chrome.ink
  ctx.font = `600 15px "Segoe UI", system-ui, sans-serif`
  ctx.fillText(session.name, margin + 26, 44)

  const meta = [
    new Date(session.startedAt).toLocaleString(lang.code),
    t('misc.export.samples', {n: session.samples, count: fmtNum(session.samples)}),
    t('misc.export.interval', {n: session.intervalMs}),
  ].join('  ·  ')
  ctx.fillStyle = chrome.mut
  ctx.font = `11px ${MONO}`
  ctx.fillText(meta, width - margin - ctx.measureText(meta).width, 25)
}
