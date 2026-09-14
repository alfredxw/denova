import assert from 'node:assert/strict';
import { test } from 'node:test';
import { readFile, mkdir } from 'node:fs/promises';
import { createRequire } from 'node:module';
import { fileURLToPath } from 'node:url';

const { chromium } = createRequire(new URL('../../web/package.json', import.meta.url))('@playwright/test');
const narrative = '[scene|The station|A mountain station at dusk.]\n[narrator] The lamp flickers.\n[cg|Lin holds a letter in the dusk.]\n[lin|smile] “Welcome back,” Lin says.';

// Network fixtures exercise the distributed iframe and HTTP client unchanged.
// They are test inputs only; the distributed game has no fake-model code path.
async function fixture(page, { adopted = false, locale = 'en-US', theme = 'dark', lostImageReply = '', lostArtSaveReply = false, streaming = false } = {}) {
  const state = { storyCalls: 0, modelCalls: 0, imageCalls: 0, imageRequests: [], imageOperations: new Map(), imageCancels: 0, records: new Map(), runs: new Map(), locale, theme, revision: 0, commit: null };
  state.snapshot = { storyId: 'story-1', branchId: 'main', title: 'The station', turns: adopted ? [{ id: 'turn-1', revision: 'source-1', narrative, user: 'Arrive', choices: ['Open the letter', 'Ask Lin'], versions: [] }] : [], branches: [{ id: 'main', title: 'Main', current: true }], hasMore: false, status: 'idle' };
  const json = (route, value, status = 200) => route.fulfill({ status, headers: { 'Content-Type': 'application/json', 'Access-Control-Allow-Origin': '*' }, body: JSON.stringify(value) });
  await page.route('https://shell.test/**', route => route.fulfill({ contentType: 'text/html', body: `<html><head><style>html,body,iframe{margin:0;width:100%;height:100%;border:0}</style></head><body><iframe src="https://game.test/index.html"></iframe><script>window.addEventListener('message',event=>{if(event.data.type==='denova:ready')event.source.postMessage({type:'denova:bootstrap',nonce:event.data.nonce,connection:{baseUrl:'https://api.test',token:'fixture-token'},context:{locale:'${locale}',theme:'${theme}',scope:{projectId:'project-1'},settings:{autoSeconds:2},setup:{}}},event.origin)});</script></body></html>` }));
  await page.route('https://game.test/**', async route => {
    const name = new URL(route.request().url()).pathname.slice(1);
    const path = name === 'client.mjs' ? 'common/client.mjs' : `galgame/${name}`;
    const contentType = name.endsWith('.mjs') ? 'text/javascript' : name.endsWith('.json') ? 'application/json' : name.endsWith('.css') ? 'text/css' : name.endsWith('.png') ? 'image/png' : 'text/html';
    await route.fulfill({ contentType, body: await readFile(new URL(`./templates/${path}`, import.meta.url)) });
  });
  await page.route('https://api.test/**', async route => {
    const request = route.request(), url = new URL(request.url()), path = url.pathname;
    if (request.method() === 'OPTIONS') return route.fulfill({ status: 204, headers: { 'Access-Control-Allow-Origin': '*', 'Access-Control-Allow-Headers': '*', 'Access-Control-Allow-Methods': '*' } });
    const body = request.postData() ? request.postDataJSON() : null;
    if (path === '/story') return json(route, state.snapshot);
    if (path === '/story/records') {
      const input = body ?? Object.fromEntries(url.searchParams), key = `${input.key}:${input.branchId ?? ''}:${input.turnId ?? ''}:${input.sourceRevision ?? ''}`;
      const record = state.records.get(key) ?? { revision: 0, schemaVersion: 0, value: null };
      if (body) {
        if (body.expectedRevision !== record.revision) return json(route, { code: 'DOCUMENT_CONFLICT', diagnostic: 'Revision conflict' }, 409);
        state.records.set(key, { revision: ++state.revision, schemaVersion: 1, value: body.value });
        if (lostArtSaveReply && body.key === 'scene' && !body.value.pendingArt && Object.keys(body.value.assets).length) {
          lostArtSaveReply = false;
          return route.abort('failed');
        }
      }
      return json(route, state.records.get(key) ?? record);
    }
    if (path === '/story/commands') {
      state.storyCalls++;
      const number = state.snapshot.turns.length + 1;
      const commit = () => {
        state.snapshot.turns.push({ id: `turn-${number}`, revision: `source-${number}`, user: body.message, narrative, choices: ['Open the letter', 'Ask Lin'], versions: [] });
        state.snapshot.status = 'idle';
      };
      if (streaming) { state.snapshot.status = 'running'; state.snapshot.operationId = 'operation-1'; state.commit = commit; }
      else commit();
      return json(route, state.snapshot);
    }
    if (path === '/story/events') return route.fulfill({ headers: { 'Content-Type': 'text/event-stream', 'Access-Control-Allow-Origin': '*' }, body: 'data: {"kind":"reset"}\n\ndata: ' + JSON.stringify({ kind: 'delta', text: '[lin|smile] Welcome' }) + '\n\n' });
    if (path === '/library/items') return json(route, { items: [{ id: 'character-1', type: 'character', name: 'A very long character name whose story stretches beyond the mountain station', briefDescription: 'From your own project library.' }], total: 1 });
    if (path === '/library/items/character-1') return json(route, { id: 'character-1', name: 'A very long character name whose story stretches beyond the mountain station', content: 'An adult traveler with a red scarf.', image: { kind: 'project', path: 'characters/traveler.png' } });
    if (path === '/images/generations') {
      state.imageRequests.push(body);
      if (!state.imageOperations.has(body.commandId)) {
        state.imageCalls++;
        const status = lostImageReply === 'running' ? 'running' : 'completed';
        state.imageOperations.set(body.commandId, { commandId: body.commandId, status, images: status === 'completed' ? [{ asset: { kind: 'generated', path: `images/${state.imageCalls}.png` } }] : [] });
      }
      if (lostImageReply) { lostImageReply = ''; return route.abort('failed'); }
      return json(route, state.imageOperations.get(body.commandId));
    }
    if (path.startsWith('/images/generations/')) {
      const result = state.imageOperations.get(path.split('/')[3]);
      if (path.endsWith('/cancel')) { state.imageCancels++; result.status = 'cancelled'; result.images = []; }
      return json(route, result);
    }
    if (path === '/assets/content') return route.fulfill({ headers: { 'Access-Control-Allow-Origin': '*', 'Content-Type': 'image/png' }, body: await readFile(new URL(`./templates/galgame/art/${url.searchParams.get("kind") === "project" ? "xu" : "station"}.png`, import.meta.url)) });
    throw new Error(`Unexpected fixture route: ${request.method()} ${path}`);
  });
  await page.goto('https://shell.test/');
  return { state, frame: page.frameLocator('iframe') };
}

test('Galgame iframe composes cast, Story, images, playback and refresh through public APIs', async () => {
  const browser = await chromium.launch({ headless: true });
  try {
    const page = await browser.newPage({ viewport: { width: 1280, height: 900 } });
    const errors = []; page.on('pageerror', error => errors.push(error.message));
    const { frame, state } = await fixture(page);
    await frame.locator('#begin').waitFor({ state: 'visible' });
    await frame.locator('#castButton').click();
    await frame.locator('#castDialog details').evaluate(element => { element.open = true; });
    await frame.locator('#libraryList button').click();
    await frame.locator('#saveCast').click();
    await frame.locator('#begin').click();
    await frame.locator('#line').filter({ hasText: 'The lamp flickers.' }).waitFor();
    assert.equal(state.storyCalls, 1); assert.equal(state.modelCalls, 0);
    await frame.locator('#next').click();
    await frame.locator('#line').filter({ hasText: 'Welcome back' }).waitFor();
    assert.equal(state.storyCalls, 1);
    assert.equal(await frame.locator('#speaker').textContent(), 'Lin');
    await frame.locator('#artButton').click();
    await frame.locator('#artList button').first().click();
    await frame.locator('#artStatus').filter({ hasText: 'Illustration saved' }).waitFor();
    assert.equal(state.imageCalls, 1);
    await frame.locator('[data-close="artDialog"]').click();
    await page.reload();
    await frame.locator('#line').filter({ hasText: 'Welcome back' }).waitFor();
    assert.equal(state.modelCalls, 0); assert.equal(state.storyCalls, 1);
    await frame.locator('#choices button').first().click();
    await frame.locator('#line').filter({ hasText: 'The lamp flickers.' }).waitFor();
    assert.equal(state.storyCalls, 2); assert.equal(state.modelCalls, 0);
    await frame.locator('#background[src^="blob:"]').waitFor();
    await mkdir(new URL('../../web/test-results/galgame-template/', import.meta.url), { recursive: true });
    await page.screenshot({ path: fileURLToPath(new URL('../../web/test-results/galgame-template/wide-dark.png', import.meta.url)), fullPage: true });
    await page.setViewportSize({ width: 390, height: 844 });
    await page.screenshot({ path: fileURLToPath(new URL('../../web/test-results/galgame-template/narrow-dark.png', import.meta.url)), fullPage: true });
    const overflow = await frame.locator('body').evaluate(element => element.scrollWidth > innerWidth);
    assert.equal(overflow, false);
    assert.deepEqual(errors, []);
  } finally { await browser.close(); }
});

test('existing Story adoption configures presentation without advancing its plot; light Chinese layout fits mobile', async () => {
  const browser = await chromium.launch({ headless: true });
  try {
    const page = await browser.newPage({ viewport: { width: 390, height: 844 } });
    const { frame, state } = await fixture(page, { adopted: true, locale: 'zh-CN', theme: 'light' });
    await frame.locator('#begin').filter({ hasText: '进入这个故事' }).waitFor();
    await frame.locator('#castButton').click();
    assert.equal(await frame.locator('#saveCast').isEnabled(), true);
    await frame.locator('[data-close="castDialog"]').click();
    await frame.locator('#begin').click();
    await frame.locator('#line').filter({ hasText: 'The lamp flickers.' }).waitFor();
    assert.equal(state.storyCalls, 0); assert.equal(state.modelCalls, 0);
    assert.equal(await frame.locator('html').getAttribute('data-theme'), 'light');
    await mkdir(new URL('../../web/test-results/galgame-template/', import.meta.url), { recursive: true });
    await page.screenshot({ path: fileURLToPath(new URL('../../web/test-results/galgame-template/narrow-light.png', import.meta.url)), fullPage: true });
    assert.equal(await frame.locator('body').evaluate(element => element.scrollWidth > innerWidth), false);
  } finally { await browser.close(); }
});

test('uncertain illustration requests retain their identity until recovered or cancelled', async t => {
  const browser = await chromium.launch({ headless: true });
  try {
    for (const scenario of ['recover', 'cancel', 'saved']) await t.test(scenario, async () => {
      const page = await browser.newPage();
      try {
        const errors = []; page.on('pageerror', error => errors.push(error.message));
        const { frame, state } = await fixture(page, { lostImageReply: scenario === 'saved' ? '' : scenario === 'cancel' ? 'running' : 'completed', lostArtSaveReply: scenario === 'saved' });
        await frame.locator('#begin').click();
        await frame.locator('#line').filter({ hasText: 'The lamp flickers.' }).waitFor();
        await frame.locator('#artButton').click();
        await frame.locator('#artList button').first().click();
        await frame.locator('#artStatus button').filter({ hasText: 'Recover request' }).waitFor();
        assert.equal(state.imageCalls, 1);
        assert.equal(await frame.locator('#artList button:enabled').count(), 0);
        assert.equal(await frame.locator('#artStatus button').filter({ hasText: /^Stop$/ }).isEnabled(), true);
        const commandId = state.imageRequests[0].commandId;
        await frame.locator('#artStatus button').filter({ hasText: scenario === 'cancel' ? /^Stop$/ : 'Recover request' }).click();
        await frame.locator('#artStatus').filter({ hasText: scenario === 'cancel' ? 'Illustration stopped' : 'Illustration saved' }).waitFor();
        assert.equal(state.imageCalls, 1);
        assert.equal(state.imageCancels, scenario === 'cancel' ? 1 : 0);
        assert.equal(state.imageRequests.length, scenario === 'saved' ? 1 : 2);
        assert.ok(state.imageRequests.every(request => request.commandId === commandId));
        assert.equal(state.records.get('scene:main:turn-1:source-1').value.pendingArt, null);
        assert.equal(await frame.locator('#artList button').first().isEnabled(), true);
        await frame.locator('#artList button').first().click();
        await frame.locator('#artStatus').filter({ hasText: 'Illustration saved' }).waitFor();
        assert.equal(state.imageCalls, 2);
        assert.notEqual(state.imageRequests.at(-1).commandId, commandId);
        assert.deepEqual(errors, []);
      } finally { await page.close(); }
    });
  } finally { await browser.close(); }
});


test('dialogue is playable before the Story commits; no presenter Agent runs', async () => {
  const browser = await chromium.launch({ headless: true });
  try {
    const page = await browser.newPage();
    const { frame, state } = await fixture(page, { streaming: true });
    await frame.locator('#begin').click();
    await frame.locator('#line').filter({ hasText: 'Welcome' }).waitFor();
    assert.equal(state.snapshot.turns.length, 0);
    assert.equal(await frame.locator('#speaker').textContent(), 'Lin');
    assert.equal(await frame.locator('#cancel').isVisible(), true);
    assert.equal(state.modelCalls, 0);
    state.commit();
    await frame.locator('#line').filter({ hasText: 'The lamp flickers.' }).waitFor();
    assert.equal(await frame.locator('#composer').isVisible(), true);
    assert.equal(state.storyCalls, 1);
    await page.reload();
    await frame.locator('#line').filter({ hasText: 'The lamp flickers.' }).waitFor();
    assert.equal(state.storyCalls, 1);
  } finally { await browser.close(); }
});
