import assert from 'node:assert/strict';
import { test } from 'node:test';
import { readFile } from 'node:fs/promises';
import { parseScene, scenePrompt, restoreSave, restoreReading, restoreScene, narrationScene, artTargets, limits } from './templates/galgame/scene.mjs';
import { host } from './templates/galgame/host.mjs';

const cast = [{ id: 'lin', name: 'Lin', profile: 'An adult lamplighter.' }, { id: 'xu', name: 'Xu', profile: 'An adult archivist.' }];
const narrative = '[scene|The station|A quiet station at dusk.]\n[narrator] The lamp flickers.\n[cg|A lamplighter holds a letter.]\n[lin|smile] “Welcome back,” Lin says.';

test('the same script produces speaker, expression and art while text is still arriving', () => {
  for (let length = 0; length <= narrative.length; length++) {
    const scene = parseScene(narrative.slice(0, length), cast, true);
    assert.ok(scene.beats.every(beat => !beat.text.startsWith('[')));
  }
  assert.deepEqual(parseScene(narrative, cast).beats, [
    { speaker: 'narrator', text: 'The lamp flickers.', expression: 'neutral', cg: '' },
    { speaker: 'lin', text: '“Welcome back,” Lin says.', expression: 'smile', cg: 'A lamplighter holds a letter.' },
  ]);
});

test('plain adopted prose remains intact without another model call', () => {
  assert.deepEqual(parseScene('An old story.\n\nA new page.', cast).beats.map(beat => beat.text), ['An old story.', 'A new page.']);
  assert.equal(parseScene('[lin|unknown] Hello', cast).beats[0].expression, 'neutral');
});

test('oversize context is rejected explicitly and the game requests short player-led exchanges', () => {
  assert.throws(() => parseScene('x'.repeat(limits.scene + 1), cast), { code: 'SCENE_TOO_LARGE' });
  assert.throws(() => scenePrompt({ cast: [{ ...cast[0], profile: 'x'.repeat(limits.profile + 1) }], locale: 'en-US', premise: '' }), { code: 'CAST_INVALID' });
  const prompt = scenePrompt({ cast, locale: 'zh-CN', premise: 'The station' });
  assert.match(prompt, /Simplified Chinese/); assert.match(prompt, /two to six/); assert.match(prompt, /do not generate an entire chapter/i);
});

test('reading state and pending operations are validated for safe refresh recovery', () => {
  const save = { version: 1, configured: true, cast, premise: 'The station', pending: { kind: 'advance', commandId: 'request-1', message: 'Open the letter' } };
  assert.deepEqual(restoreSave(structuredClone(save)), save);
  assert.throws(() => restoreSave({ ...save, cast: [cast[0], cast[0]] }), { code: 'SAVE_INVALID' });
  const position = { version: 1, turnId: 'turn-1', sourceRevision: 'rev-1', beat: 1 };
  assert.deepEqual(restoreReading(structuredClone(position)), position);
  assert.throws(() => restoreReading({ ...position, beat: -1 }), { code: 'SAVE_INVALID' });
  assert.throws(() => restoreSave({ ...save, pending: { kind: 'advance', commandId: 'request-1' } }), { code: 'SAVE_INVALID' });
  const record = { version: 1, runId: 'run-1', pending: null, pendingArt: { commandId: 'art-1', key: 'portrait:lin:smile', prompt: 'An adult lamplighter smiles.', kind: 'portrait' }, assets: { 'portrait:lin:neutral': { kind: 'generated', path: 'images/lin.png' } } };
  assert.deepEqual(restoreScene(structuredClone(record)), record);
  assert.throws(() => restoreScene({ ...record, assets: { bad: { kind: 'generated', path: 'C:/Users/name/image.png' } } }), { code: 'SAVE_INVALID' });
  assert.throws(() => restoreScene({ ...record, assets: { bad: { kind: 'project', path: '../image.png' } } }), { code: 'SAVE_INVALID' });
});

test('art belongs to its source turn and expression at the current reading step', () => {
  const scene = parseScene(narrative, cast);
  const first = artTargets(scene, cast, 'turn-1', 0), second = artTargets(scene, cast, 'turn-1', 1);
  assert.deepEqual(first.map(item => item.key), ['background:turn-1', 'portrait:lin:neutral', 'portrait:xu:neutral']);
  assert.deepEqual(second.map(item => item.key), ['background:turn-1', 'portrait:lin:smile', 'portrait:xu:neutral', 'cg:turn-1:1']);
  assert.match(second[1].prompt, /Expression: smile/);
});

test('HTTP adapter preserves command IDs and exact turn revision on record writes', async () => {
  const calls = [];
  const api = host({ context: { scope: { projectId: 'project-1' } }, async request(path, options) { calls.push({ path, ...(options ? { method: options.method, body: JSON.parse(options.body) } : {}) }); return {}; } });
  await api.command({ kind: 'advance', commandId: 'intent-1', message: 'Open the letter' });
  await api.save('scene', { revision: 8 }, { version: 1 }, { id: 'turn-1', revision: 'source-2' }, 'branch-1');
  await api.image({ commandId: 'art-1', prompt: 'A portrait.', kind: 'portrait' });
  assert.deepEqual(calls, [
    { path: '/story/commands', method: 'POST', body: { kind: 'advance', commandId: 'intent-1', message: 'Open the letter' } },
    { path: '/story/records', method: 'PUT', body: { key: 'scene', turnId: 'turn-1', sourceRevision: 'source-2', branchId: 'branch-1', expectedRevision: 8, schemaVersion: 1, value: { version: 1 } } },
    { path: '/images/generations', method: 'POST', body: { commandId: 'art-1', modelSlot: 'illustrator', prompt: 'A portrait.', aspectRatio: '2:3' } },
  ]);
});

test('Story stream delivers fragmented UTF-8 before settlement and retracts provisional text', async () => {
  const frames = [{ kind: 'reset' }, { kind: 'delta', text: '灯亮了。' }, { kind: 'reset' }, { kind: 'delta', text: 'The lamp flickers.' }, { kind: 'settled' }];
  const encoded = new TextEncoder().encode(frames.map(event => `data: ${JSON.stringify(event)}\r\n\r\n`).join(''));
  const response = new Response(new ReadableStream({ start(controller) { for (let i = 0; i < encoded.length; i += 3) controller.enqueue(encoded.slice(i, i + 3)); controller.close(); } }));
  const received = [];
  await host({ request: async () => response }).followStory('operation-1', event => received.push(event));
  assert.deepEqual(received, frames.slice(0, -1));
});

test('disconnected stream cannot claim completion', async () => {
  await assert.rejects(host({ request: async () => new Response('data: {"kind":"delta","text":"partial"}\n\n') }).followStory('operation-1', () => {}), /disconnected/);
});

test('sample UI locales cover the same independent resources', async () => {
  const locales = await Promise.all(['en-US', 'zh-CN'].map(locale => readFile(new URL(`./templates/galgame/locales/${locale}.json`, import.meta.url), 'utf8').then(JSON.parse)));
  const keys = (value, prefix = '') => Object.entries(value).flatMap(([key, item]) => typeof item === 'object' ? keys(item, `${prefix}${key}.`) : `${prefix}${key}`).sort();
  assert.deepEqual(keys(locales[0]), keys(locales[1]));
  const html = await readFile(new URL('./templates/galgame/index.html', import.meta.url), 'utf8');
  for (const [, key] of html.matchAll(/data-text="([^"]+)"/gu)) assert.equal(typeof locales[0][key], 'string', `Missing UI key ${key}`);
});
