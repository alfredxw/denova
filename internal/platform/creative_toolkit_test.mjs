import assert from 'node:assert/strict';
import { test } from 'node:test';
import { mkdtemp, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { invoke } from './templates/http-tool/tools.mjs';

test('creative tools isolate notes, deduplicate concurrent retries, and retain individual batch successes', async t => {
  const directory = await mkdtemp(join(tmpdir(), 'denova-toolkit-'));
  t.after(() => rm(directory, { recursive: true, force: true }));
  const writing = { dataDir: join(directory, 'writing'), context: { settings: {} } };
  const game = { ...writing, dataDir: join(directory, 'game') };
  const input = { requestId: 'note-1', text: 'A letter at the station' };
  const [first, retry] = await Promise.all([invoke(writing, '/tools/notes-save', input), invoke(writing, '/tools/notes-save', input)]);
  assert.deepEqual(retry, first);
  await assert.rejects(invoke(writing, '/tools/notes-save', { ...input, text: 'Changed' }), /conflicts/);
  assert.deepEqual((await invoke(writing, '/tools/notes-read', {})).data.notes, [first.data]);
  assert.deepEqual((await invoke(game, '/tools/notes-read', {})).data.notes, []);
  assert.deepEqual((await invoke(writing, '/tools/clean', { items: ['  e\u0301 \n line  ', 3, 'good'] })).data.items, [
    { index: 0, text: 'é\n line' }, { index: 1, error: 'Expected text of at most 24000 characters' }, { index: 2, text: 'good' },
  ]);
  assert.deepEqual((await invoke(writing, '/tools/count', { text: '你🙂 ' })).data, { count: 3 });
  writing.context.settings.ignoreWhitespace = true;
  assert.deepEqual((await invoke(writing, '/tools/count', { text: '你🙂 ' })).data, { count: 2 });
});

test('host resource requests use scoped credentials and preserve image request identity', async t => {
  const calls = [];
  t.mock.method(globalThis, 'fetch', async (url, options) => {
    calls.push({ url, options });
    return new Response(JSON.stringify({ commandId: 'art-1', status: 'running' }), { status: 202 });
  });
  const bootstrap = { connection: { baseUrl: 'http://127.0.0.1:1', token: 'scoped' } };
  await invoke(bootstrap, '/tools/library', { query: '林 & 灯' });
  assert.equal(new URL(calls[0].url).searchParams.get('query'), '林 & 灯');
  for (let i = 0; i < 2; i++) await invoke(bootstrap, '/tools/illustrate', { commandId: 'art-1', prompt: 'A station' });
  assert.equal(calls[1].options.headers.Authorization, 'Bearer scoped');
  assert.deepEqual(calls[1], calls[2]);
});
