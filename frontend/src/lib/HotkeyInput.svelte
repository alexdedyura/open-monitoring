<script>
  // A capture field rather than a text box: the backend accepts one spelling of
  // a combination, and asking the user to learn it would be a worse deal than
  // asking them to press the keys.
  import {t} from './i18n.svelte.js'

  let {value = '', live = true, onchange} = $props()

  let listening = $state(false)

  // What internal/hotkey's parse() can turn into a virtual-key code: letters,
  // digits and F1-F24. Offering anything else here would hand the backend a
  // combination it rejects, and the user a shortcut that never fires.
  //
  // The name comes from event.code, not event.key: key is what the layout
  // produces, so Ctrl+Alt+H on a Cyrillic layout would arrive as Ctrl+Alt+Р.
  // code names the physical key, which is what a virtual-key code means.
  function keyName(code) {
    if (/^Key[A-Z]$/.test(code)) return code.slice(3)
    if (/^Digit\d$/.test(code)) return code.slice(5)
    if (/^F([1-9]|1\d|2[0-4])$/.test(code)) return code
    return null
  }

  function capture(e) {
    // Also stops Space and Enter from activating the button we are listening on.
    e.preventDefault()

    if (e.key === 'Escape') {
      listening = false
      return
    }
    const key = keyName(e.code)
    if (!key) return // a modifier on its own, or a key that cannot be registered

    const mods = []
    if (e.ctrlKey) mods.push('Ctrl')
    if (e.altKey) mods.push('Alt')
    if (e.shiftKey) mods.push('Shift')
    if (e.metaKey) mods.push('Win')
    // A bare key is registered machine-wide too, so it would stop reaching every
    // other application, the game included. config.json still allows one for
    // anybody who insists; this field does not hand it to them by accident.
    if (!mods.length) return

    listening = false
    onchange([...mods, key].join('+'))
  }
</script>

<button
  class="min-w-[9rem] rounded-md border px-2.5 py-1.5 text-center font-mono text-xs {listening
    ? 'border-ink bg-card2 text-ink'
    : 'border-line bg-card2 text-ink hover:border-ink2'}"
  onclick={() => (listening = !listening)}
  onkeydown={listening ? capture : null}
  onblur={() => (listening = false)}
>
  {listening ? t('app.hotkey.listening') : value}
</button>

{#if !live && !listening}
  <div class="mt-1 font-mono text-[10px] text-rec">{t('app.hotkey.taken')}</div>
{/if}
