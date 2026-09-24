import { serve, readJSON } from './runtime.mjs';
import { invoke } from './tools.mjs';
await serve(async (request, response, bootstrap) => {
  if (request.method !== 'POST') { response.writeHead(404).end(); return; }
  const result = await invoke(bootstrap, request.url, await readJSON(request));
  response.writeHead(200, { 'Content-Type': 'application/json' });
  response.end(JSON.stringify(result));
});
