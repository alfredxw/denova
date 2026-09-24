import { createInterface } from 'node:readline';
import { createServer } from 'node:http';
import { timingSafeEqual } from 'node:crypto';

// Credentials arrive through stdin, never argv, URLs or log output.
export async function serve(handler) {
  const input = createInterface({ input: process.stdin });
  const bootstrap = await new Promise((resolve, reject) => {
    input.once('line', (line) => { try { resolve(JSON.parse(line)); } catch (error) { reject(error); } });
    input.once('close', () => reject(new Error('Bootstrap pipe closed')));
  });
  if (bootstrap.protocol !== 'denova-runtime-v1') throw new Error('Unsupported runtime protocol');
  const expected = Buffer.from(`Bearer ${bootstrap.hostToken}`);
  const server = createServer(async (request, response) => {
    const actual = Buffer.from(request.headers.authorization || '');
    if (actual.length !== expected.length || !timingSafeEqual(actual, expected)) {
      response.writeHead(403).end(); return;
    }
    if (request.method === 'GET' && request.url === '/__denova/ready') {
      response.writeHead(200).end('denova-runtime-v1'); return;
    }
    try { await handler(request, response, bootstrap); }
    catch (error) {
      console.error('Backend request failed:', error.message);
      if (!response.headersSent) response.writeHead(500, { 'Content-Type': 'application/json' });
      response.end(JSON.stringify({ code: 'RUNTIME_FAILED', messageKey: 'errors.requestFailed', diagnostic: 'Backend request failed' }));
    }
  });
  input.on('line', (line) => {
    try { if (JSON.parse(line).type === 'shutdown') { server.close(); server.closeAllConnections(); input.close(); } }
    catch { console.error('Invalid runtime control message'); }
  });
  input.on('close', () => { server.close(); server.closeAllConnections(); });
  await new Promise((resolve, reject) => { server.once('error', reject); server.listen(0, '127.0.0.1', resolve); });
  console.log(JSON.stringify({ type: 'ready', protocol: 'denova-runtime-v1', port: server.address().port }));
}

export async function readJSON(request) {
  const chunks = []; let size = 0;
  for await (const chunk of request) { size += chunk.length; if (size > 1048576) throw new Error('Request too large'); chunks.push(chunk); }
  return JSON.parse(Buffer.concat(chunks).toString('utf8'));
}
