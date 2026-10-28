import { defineConfig } from 'vite';

// Preserve the browser-facing Host so the API can compare it with Origin.
// Vite's string proxy shorthand enables changeOrigin and breaks that comparison.
const apiProxy = () => ({
  '/api': { target: 'http://127.0.0.1:8080', changeOrigin: false },
});

export default defineConfig({
  server: { port: 5173, strictPort: true, proxy: apiProxy() },
  preview: { port: 5173, strictPort: true, proxy: apiProxy() },
});
