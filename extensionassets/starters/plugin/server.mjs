import { readJSON, serve } from './runtime.mjs';

// Replace this minimal handler and its tool definition with your plugin's behavior.
// The host handles authentication, lifecycle and tool-input validation.
await serve(async (request, response) => {
  if (request.method === 'POST' && request.url === '/tools/echo') {
    const { text } = await readJSON(request);
    response.writeHead(200, { 'Content-Type': 'application/json' });
    response.end(JSON.stringify({ content: text, data: { text } }));
    return;
  }
  response.writeHead(404, { 'Content-Type': 'application/json' });
  response.end(JSON.stringify({ code: 'NOT_FOUND', messageKey: 'errors.notFound', diagnostic: 'Unknown plugin endpoint' }));
});
