// The renderer-only build of the demo window (src/renderer/demo): its own entry and its own output, outside out/.
// Not used by electron-vite; the product build never sees it.
import { resolve } from 'node:path'
import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

export default defineConfig({
  root: resolve(__dirname, 'src/renderer'),
  // relative, so that the page loads from file:// in the capture script
  base: './',
  plugins: [react()],
  build: {
    outDir: resolve(__dirname, 'out-demo'),
    emptyOutDir: true,
    rollupOptions: { input: resolve(__dirname, 'src/renderer/demo/demo.html') }
  }
})
