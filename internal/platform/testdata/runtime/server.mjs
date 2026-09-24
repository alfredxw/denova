// Deliberately small protocol stub. It does not import any product example.
import { createInterface } from 'node:readline';
import { createServer } from 'node:http';
import { readFile, writeFile } from 'node:fs/promises';
import { join } from 'node:path';

const input = createInterface({ input: process.stdin });
input.once('line', line => {
  const bootstrap = JSON.parse(line);
  const json = (response, value) => response.writeHead(200, { 'Content-Type': 'application/json' }).end(JSON.stringify(value));
  const server = createServer(async (request, response) => {
    if (request.headers.authorization !== `Bearer ${bootstrap.hostToken}`) return response.writeHead(403).end();
    try {
      if (request.url === '/__denova/ready') return response.end('denova-runtime-v1');
      if (request.method === 'POST' && request.url === '/tools/probe') {
        for await (const _ of request) { /* Drain the validated request body. */ }
        return json(response, { content: 'Probe completed.', data: { value: bootstrap.context.settings.enabled ? 2 : 3 } });
      }
      const path = join(bootstrap.dataDir, 'state.json');
      if (request.url === '/state' || request.url === '/increment') {
        let value = 0;
        try { value = JSON.parse(await readFile(path, 'utf8')).value; }
        catch (error) { if (error.code !== 'ENOENT') throw error; }
        if (request.method === 'POST' && request.url === '/increment') await writeFile(path, JSON.stringify({ value: ++value }));
        return json(response, { value });
      }
      if (request.url === '/') return response.end('Runtime probe');
      response.writeHead(404).end();
    } catch (error) { console.error(error); response.writeHead(500).end(); }
  });
  const close = () => { server.close(); server.closeAllConnections(); input.close(); };
  input.on('line', close);
  input.on('close', () => { server.close(); server.closeAllConnections(); });
  server.listen(0, '127.0.0.1', () => console.log(JSON.stringify({ type: 'ready', protocol: 'denova-runtime-v1', port: server.address().port })));
});
