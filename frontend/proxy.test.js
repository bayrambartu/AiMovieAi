import { test } from 'node:test';
import assert from 'node:assert/strict';
import http from 'node:http';
import { createServer, preview } from 'vite';
import config from './vite.config.js';

for (const mode of ['server', 'preview']) {
  test(`${mode} proxy preserves Host for same-origin account requests`, async () => {
    const received = [];
    const backend = http.createServer((req, res) => {
      received.push({ host: req.headers.host, origin: req.headers.origin, path: req.url });
      const sameOrigin = new URL(req.headers.origin).host === req.headers.host;
      res.writeHead(sameOrigin ? 400 : 403, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({ error: sameOrigin ? 'validation reached' : 'origin rejected' }));
    });
    await new Promise(resolve => backend.listen(0, '127.0.0.1', resolve));
    let frontend;
    try {
      const configured = config[mode].proxy['/api'];
      const proxy = { '/api': { ...configured, target: `http://127.0.0.1:${backend.address().port}` } };
      const options = { host: '127.0.0.1', port: 0, strictPort: true, proxy };
      frontend = mode === 'server'
        ? await createServer({ configFile: false, server: options })
        : await preview({ configFile: false, preview: options });
      // Vite's listen(0) falls back to 5173; use the HTTP server for an OS-assigned test port.
      if (mode === 'server') await new Promise(resolve => frontend.httpServer.listen(0, '127.0.0.1', resolve));
      const host = `127.0.0.1:${frontend.httpServer.address().port}`;
      const origin = `http://${host}`;
      const register = requestOrigin => fetch(`${origin}/api/auth/register`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', Origin: requestOrigin, 'Sec-Fetch-Site': 'same-origin' },
        body: '{}',
      });
      const valid = await register(origin);
      assert.equal(valid.status, 400);
      assert.equal((await valid.json()).error, 'validation reached');
      assert.deepEqual(received[0], { host, origin, path: '/api/auth/register' });
      const crossOrigin = await register('https://evil.example');
      assert.equal(crossOrigin.status, 403);
      await crossOrigin.text();
    } finally {
      if (frontend) {
        if (mode === 'server') await frontend.close();
        else await new Promise(resolve => { frontend.httpServer.closeAllConnections(); frontend.httpServer.close(resolve); });
      }
      await new Promise(resolve => { backend.closeAllConnections(); backend.close(resolve); });
    }
  });
}
