import { readFile, appendFile, mkdir } from 'node:fs/promises';
import { join } from 'node:path';

// The host supplies an already scoped dataDir. No user-controlled path is used.
// Append-only note records preserve older content; stable request IDs deduplicate retries.
let writes = Promise.resolve();
export async function invoke(bootstrap, path, input) {
  const result = data => ({ content: JSON.stringify(data), data });
  const host = async (route, body) => {
    const response = await fetch(bootstrap.connection.baseUrl + route, {
      method: body ? 'POST' : 'GET',
      headers: { Authorization: `Bearer ${bootstrap.connection.token}`, 'Content-Type': 'application/json' },
      ...(body ? { body: JSON.stringify(body) } : {}),
    });
    const data = await response.json();
    if (!response.ok) return { error: { code: data.code, messageKey: data.messageKey } };
    return data;
  };
  const notes = async () => {
    let data;
    try { data = await readFile(join(bootstrap.dataDir, 'notes.jsonl'), 'utf8'); }
    catch (error) { if (error.code === 'ENOENT') return []; throw error; }
    if (Buffer.byteLength(data) > 1048576) throw new Error('Note history exceeds the 1 MiB example read capacity');
    return data.split('\n').filter(Boolean).map(line => JSON.parse(line));
  };
  switch (path) {
    case '/tools/count': {
      const count = Array.from(bootstrap.context.settings.ignoreWhitespace ? input.text.replace(/\s/gu, '') : input.text).length;
      return { content: `Character count: ${count}`, data: { count } };
    }
    case '/tools/clean':
      return result({ items: input.items.map((text, index) => typeof text !== 'string' || text.length > 24000
        ? { index, error: 'Expected text of at most 24000 characters' }
        : { index, text: text.normalize('NFC').split('\n').map(line => line.trimEnd()).join('\n').trim() }) });
    case '/tools/notes-read': return result({ notes: await notes() });
    case '/tools/notes-save': {
      const task = writes.then(async () => {
        const existing = await notes(), prior = existing.find(note => note.requestId === input.requestId);
        if (prior) {
          if (prior.text !== input.text) throw new Error('Note requestId conflicts with existing content');
          return result(prior);
        }
        const note = { requestId: input.requestId, text: input.text, createdAt: new Date().toISOString() };
        if (Buffer.byteLength(existing.map(item => JSON.stringify(item) + '\n').join('') + JSON.stringify(note) + '\n') > 1048576) throw new Error('Note history is full; export the scoped notes before adding more');
        await mkdir(bootstrap.dataDir, { recursive: true });
        await appendFile(join(bootstrap.dataDir, 'notes.jsonl'), JSON.stringify(note) + '\n', { encoding: 'utf8', flush: true });
        return result(note);
      });
      writes = task.catch(() => {}); return task;
    }
    case '/tools/library': return result(await host(`/library/items?${new URLSearchParams({ query: input.query, limit: '20' })}`));
    case '/tools/illustrate': return result(await host('/images/generations', { commandId: input.commandId, modelSlot: 'illustrator', prompt: input.prompt, aspectRatio: '16:9' }));
    default: throw new Error('Unsupported creative tool');
  }
}
