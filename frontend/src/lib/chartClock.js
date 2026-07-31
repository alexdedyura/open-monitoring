// One animation-frame loop for every live chart on screen.
//
// The dashboard shows ten streaming charts. Ten independent requestAnimationFrame
// loops would each wake the page on every display refresh and each keep running
// after their chart went away; one shared loop wakes once, and stops the moment
// the last subscriber unsubscribes. (rAF already idles while the window is
// minimised or hidden, so there is nothing extra to do about that.)

const subs = new Set()
let frame = 0

function tick() {
  frame = requestAnimationFrame(tick)
  for (const fn of subs) fn()
}

// onFrame registers a callback to run once per display refresh and returns the
// unsubscribe function, so it drops straight into a Svelte effect's teardown.
export function onFrame(fn) {
  subs.add(fn)
  if (!frame) frame = requestAnimationFrame(tick)

  return () => {
    subs.delete(fn)
    if (!subs.size) {
      cancelAnimationFrame(frame)
      frame = 0
    }
  }
}
