import en from './lang.en.js'
import ru from './lang.ru.js'

// The whole i18n layer. A library would bring a store abstraction, an async
// loader and an ICU parser; two catalogues and Intl.PluralRules cover what this
// app actually needs, and the catalogues ship in the bundle anyway — there is
// nothing to load asynchronously in a desktop app that is one file.
//
// THE REACTIVITY RULE, which is the whole trick: `t()` reads `lang.code`, a
// module-scope $state. That makes it reactive only where Svelte is tracking —
// inside a template expression, a $derived, or a $effect. A `const x = t('a.b')`
// at the top of a <script> runs ONCE and freezes in whatever language was
// active at mount. So: never call t() at module or script scope. Anything that
// looks like a constant table of labels holds KEYS, and the template translates
// them at the point of use (see metricDefs.js, chartDefs.js).

const CATALOGUES = {en, ru}

export const lang = $state({
  code: 'en', // resolved: never 'auto'
  chosen: 'auto', // what the user picked: auto | en | ru
})

// resolve turns the setting plus whatever Windows reports into a real language.
// The OS string arrives as a BCP-47 tag ("ru-RU"), so only the primary subtag
// is meaningful here.
function resolve(chosen, osLang) {
  if (chosen === 'en' || chosen === 'ru') return chosen
  return String(osLang || '').toLowerCase().startsWith('ru') ? 'ru' : 'en'
}

export function setLang(chosen, osLang) {
  lang.chosen = chosen || 'auto'
  lang.code = resolve(lang.chosen, osLang)
  // Screen readers and the browser's own hyphenation follow this, and it costs
  // one line to keep honest.
  document.documentElement.lang = lang.code
}

// PluralRules is per-language and worth caching: Russian needs one/few/many and
// the lookup is not free at the rate a live table re-renders.
const plurals = {}
function pluralOf(code, n) {
  plurals[code] ??= new Intl.PluralRules(code)
  return plurals[code].select(n)
}

// t(key) or t(key, {n: 3, name: 'x'}).
//
// A value in the catalogue is either a string, or an object of plural forms
// keyed the way Intl.PluralRules names them. Placeholders are {name}.
export function t(key, params) {
  const code = lang.code
  let value = CATALOGUES[code]?.[key]

  // English is the fallback rather than the key itself: a missing Russian
  // string should read as an untranslated app, not as a broken one.
  if (value === undefined) {
    value = CATALOGUES.en[key]
    if (value === undefined) {
      if (import.meta.env.DEV) console.warn('[i18n] missing key:', key)
      return key
    }
  }

  if (typeof value === 'object') {
    const n = Number(params?.n ?? 0)
    value = value[pluralOf(code, n)] ?? value.other ?? ''
  }

  if (!params) return value
  return value.replace(/\{(\w+)\}/g, (m, name) =>
    params[name] === undefined ? m : String(params[name]),
  )
}

// Numbers follow the active locale — Russian uses a comma for the decimal mark
// and a thin space for thousands, and a monitoring app full of English decimal
// points reads as untranslated even when every word around it is right.
//
// Unit SYMBOLS deliberately do not: °C, W, MHz, GB and MB/s are SI or de-facto
// international, and Russian technical writing keeps them Latin.
export function fmtNum(v, digits = 0) {
  if (v == null || !isFinite(v)) return '—'
  return new Intl.NumberFormat(lang.code, {
    minimumFractionDigits: digits,
    maximumFractionDigits: digits,
  }).format(v)
}
