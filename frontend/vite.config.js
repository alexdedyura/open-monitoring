import {defineConfig} from 'vite'
import {svelte} from '@sveltejs/vite-plugin-svelte'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig({
  plugins: [svelte(), tailwindcss()],
  server: {
    // The About tab inlines ../README.md (?raw); allow the dev server to read
    // one level above the frontend root.
    fs: {allow: ['..']},
  },
})
