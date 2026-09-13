import { serve, readJSON } from './runtime.mjs';
await serve(async (request, response, bootstrap) => {
  if (request.method !== 'POST' || request.url !== '/tools/count') { response.writeHead(404).end(); return; }
  const { text } = await readJSON(request);
  if (typeof text !== 'string') { response.writeHead(400).end(); return; }
  const count = Array.from(bootstrap.context.settings.ignoreWhitespace ? text.replace(/\s/gu, "") : text).length;
  response.writeHead(200, { 'Content-Type': 'application/json' });
  response.end(JSON.stringify({ content: `Character count: ${count}`, data: { count } }));
});
