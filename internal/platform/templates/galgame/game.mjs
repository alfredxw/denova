import { connect } from './client.mjs';
import { host, nextPoll } from './host.mjs';
import { parseScene, scenePrompt, restoreSave, restoreReading, restoreScene, narrationScene, artTargets, limits } from './scene.mjs';
import { elements, localize, renderStage, renderCast, renderHistory, renderArt, assetKey } from './stage.mjs';

const translations = Object.fromEntries(await Promise.all(['zh-CN', 'en-US'].map(async locale => [locale, await (await fetch(`./locales/${locale}.json`)).json()])));
const client = await connect(), api = host(client);
let t = translations[client.context.locale], snapshot = null, gameRecord = { revision: 0, value: null };
let readingRecord = { revision: 0, value: null };
let turn = null, scene = null, beat = 0, busy = true, ready = false, auto = false, statusKey = 'loading', recoverable = false;
let streaming = false, liveText = '', artJob = null, artStatus = '', draftCast = [], library = [], libraryOffset;
let gameWrites = Promise.resolve(), autoTimer;
const records = new Map(), sceneWrites = new Map(), urls = new Map(), assetLoads = new Map();
const defaultCast = () => ['lin', 'xu'].map(id => ({ id, name: t.defaults[id].name, tagline: t.defaults[id].tagline, profile: translations['en-US'].defaults[id].profile }));
const initialGame = () => ({ version: 1, configured: false, cast: defaultCast(), premise: t.defaults.premise, pending: null });
const game = () => gameRecord.value ?? initialGame();
const identity = source => `${snapshot.branchId}:${source.id}:${source.revision}`;
const entry = source => records.get(identity(source));
const currentAssets = () => turn ? entry(turn)?.value.assets ?? {} : {};
const targets = () => artTargets(scene, game().cast, turn?.id, beat);

function render() {
  t = translations[client.context.locale]; localize(t, client.context);
  renderStage(t, { scene, turn, adopting: !!snapshot?.turns.length && !game().configured, cast: game().cast, beat, ready, busy, atLatest: !turn || turn.id === snapshot?.turns.at(-1)?.id, assets: currentAssets(), urls, statusKey, auto, streaming, recoverable: recoverable || snapshot?.status === 'suspended', turnIndex: snapshot?.turns.findIndex(item => item.id === turn?.id) ?? 0, onAction: advance, onLatest: () => showTurn(snapshot.turns.at(-1)) });
  elements.cancel.hidden = !['running', 'compacting'].includes(snapshot?.status);
  if (snapshot?.status === 'suspended') elements.retry.textContent = t.resume;
  if (elements.artDialog.open) renderStudio();
}

function report(error, fallback = 'failed') {
  console.error('Visual novel operation failed:', error.code ?? error.message);
  statusKey = ['DOCUMENT_CONFLICT', 'STORY_CONFLICT', 'STALE_REVISION'].includes(error.code) ? 'conflict' : fallback;
  recoverable = true;
}

function writeGame(update) {
  const task = gameWrites.then(async () => { gameRecord = await api.save('game', gameRecord, update(game())); });
  gameWrites = task.catch(() => {}); return task;
}

function writeScene(source, branchId, update) {
  const key = `${branchId}:${source.id}:${source.revision}`;
  const task = (sceneWrites.get(key) ?? Promise.resolve()).then(async () => {
    const record = records.get(key);
    records.set(key, await api.save('scene', record, update(record.value), source, branchId));
  });
  sceneWrites.set(key, task.catch(() => {})); return task;
}

async function loadAssets() {
  const assets = [...Object.values(currentAssets()), ...game().cast.map(item => item.image).filter(Boolean)];
  const retained = new Set(assets.map(assetKey));
  for (const [key, url] of urls) if (!retained.has(key)) { URL.revokeObjectURL(url); urls.delete(key); }
  await Promise.all(assets.map(async asset => {
    const key = assetKey(asset);
    if (urls.has(key)) return;
    if (!assetLoads.has(key)) assetLoads.set(key, api.asset(asset).then(url => urls.set(key, url)).catch(error => console.warn('Visual novel asset unavailable:', error.code ?? error.message)).finally(() => assetLoads.delete(key)));
    await assetLoads.get(key);
  }));
}

async function loadScene(source) {
  const branchId = snapshot.branchId, key = identity(source);
  const parsed = parseScene(source.narrative, game().cast);
  const record = await api.record('scene', source, branchId);
  if (record.value) record.value = restoreScene(record.value);
  else {
    const previous = snapshot.turns[snapshot.turns.findIndex(item => item.id === source.id) - 1];
    const prior = previous ? records.get(identity(previous)) ?? await api.record('scene', previous, branchId) : null;
    const assets = Object.fromEntries(Object.entries(prior?.value?.assets ?? {}).filter(([name]) => name.startsWith('portrait:')));
    if (previous && (!parsed.background || parsed.background === parseScene(previous.narrative, game().cast).background)) {
      const background = prior?.value?.assets[`background:${previous.id}`];
      if (background) assets[`background:${source.id}`] = background;
    }
    record.value = { version: 1, assets, pendingArt: null };
  }
  records.set(key, record);
  return parsed;
}

async function showTurn(source) {
  if (!source) { turn = null; scene = null; return; }
  stopAuto(); busy = true; turn = source; scene = null; render();
  try {
    scene = await loadScene(source);
    const position = readingRecord.value;
    beat = position?.turnId === source.id && position.sourceRevision === source.revision ? Math.min(position.beat, Math.max(0, scene.beats.length - 1)) : 0;
    if (position?.turnId !== source.id || position.sourceRevision !== source.revision || position.beat !== beat) readingRecord = await api.save('reading', readingRecord, { version: 1, turnId: source.id, sourceRevision: source.revision, beat });
    await loadAssets();
    const pending = entry(source)?.value.pendingArt;
    if (pending && artJob?.status !== 'running') void generateArt(pending, source, snapshot.branchId);
    else if (!pending && artJob?.status === 'unresolved' && artJob.source.id === source.id && artJob.source.revision === source.revision && artJob.branchId === snapshot.branchId) artJob = null;
  } catch (error) {
    if (error.code === 'SCENE_TOO_LARGE') statusKey = 'sceneTooLarge'; else report(error);
    scene = narrationScene(source.narrative); beat = 0;
  } finally { busy = false; render(); }
}

async function followStory() {
  streaming = true; liveText = '';
  try {
    while (['running', 'compacting'].includes(snapshot.status)) {
      const operationId = snapshot.operationId;
      try {
        await api.followStory(operationId, event => {
          if (event.kind === 'reset') liveText = '';
          else if (event.kind === 'delta') liveText += event.text;
          else return;
          scene = parseScene(liveText, game().cast, true);
          // Keep the reader's position while new lines arrive. The active line
          // grows directly from model deltas; no simulated typing delay.
          beat = Math.min(beat, Math.max(0, scene.beats.length - 1));
          render();
        });
      } catch (error) {
        if (error.code === 'SCENE_TOO_LARGE') throw error;
        console.warn('Story stream interrupted; recovering snapshot:', error.code ?? error.message);
        await nextPoll();
      }
      snapshot = await api.snapshot(); render();
    }
  } finally { streaming = false; }
  if (snapshot.status === 'suspended') statusKey = 'incomplete';
}

async function performCommand(command) {
  stopAuto(); busy = true; recoverable = false; statusKey = 'writing'; render();
  try {
    await writeGame(value => ({ ...value, configured: true, formatInitialized: command.kind === 'advance' || value.formatInitialized === true, pending: command }));
    snapshot = await api.command(command); beat = 0; render(); await followStory();
    await writeGame(value => ({ ...value, pending: null }));
    statusKey = snapshot.status === 'suspended' ? 'incomplete' : 'ready';
    const position = beat;
    await showTurn(snapshot.turns.at(-1));
    if (scene?.beats.length) {
      beat = Math.min(position, scene.beats.length - 1);
      readingRecord = await api.save('reading', readingRecord, { version: 1, turnId: turn.id, sourceRevision: turn.revision, beat });
    }
  } catch (error) { report(error); }
  finally { busy = false; render(); }
}

async function advance(message) {
  if (!ready || busy || game().pending || snapshot.status === 'suspended') return;
  if (!game().configured && snapshot.turns.length) {
    busy = true; render();
    try { await writeGame(value => ({ ...value, configured: true })); await showTurn(snapshot.turns.at(-1)); }
    catch (error) { report(error); }
    finally { busy = false; render(); }
    return;
  }
  if (snapshot.turns.length && !message?.trim()) return;
  if (!game().formatInitialized) message = scenePrompt({ cast: game().cast, locale: client.context.locale, premise: game().premise }) + (snapshot.turns.length ? `\nContinue the existing story without restarting it. Player action: ${JSON.stringify(message)}` : '');
  if (!message?.trim()) return;
  elements.input.value = '';
  await performCommand({ kind: 'advance', commandId: crypto.randomUUID(), message: message.trim(), locale: client.context.locale, branchId: snapshot.branchId });
}

async function initialize() {
  stopAuto(); busy = true; ready = false; recoverable = false; statusKey = 'loading'; render();
  try {
    [snapshot, gameRecord, readingRecord] = await Promise.all([api.snapshot(), api.record('game'), api.record('reading')]);
    readingRecord.value = restoreReading(readingRecord.value);
    if (gameRecord.value) gameRecord.value = restoreSave(gameRecord.value);
    else await writeGame(value => value);
    ready = true;
    if (game().pending) snapshot = await api.command(game().pending);
    await followStory();
    if (game().pending) await writeGame(value => ({ ...value, pending: null }));
    statusKey = snapshot.status === 'suspended' ? 'incomplete' : 'ready';
    const saved = snapshot.turns.find(item => item.id === readingRecord.value?.turnId && item.revision === readingRecord.value.sourceRevision);
    if (game().configured) await showTurn(saved ?? snapshot.turns.at(-1));
  } catch (error) { report(error); }
  finally { busy = false; render(); }
}

async function moveLine(offset) {
  if (!scene || (busy && !streaming)) return;
  if (streaming) { beat = Math.max(0, Math.min(scene.beats.length - 1, beat + offset)); render(); return; }
  const next = Math.max(0, Math.min(scene.beats.length - 1, beat + offset));
  if (next === beat) { stopAuto(); return; }
  busy = true;
  try {
    readingRecord = await api.save('reading', readingRecord, { version: 1, turnId: turn.id, sourceRevision: turn.revision, beat: next });
    beat = next; statusKey = 'saved';
    await loadAssets();
  } catch (error) { stopAuto(); report(error); }
  finally { busy = false; render(); scheduleAuto(); }
}

function stopAuto() { auto = false; clearTimeout(autoTimer); }
function scheduleAuto() {
  clearTimeout(autoTimer);
  if (!auto || !scene) return;
  if (beat === scene.beats.length - 1) { stopAuto(); render(); return; }
  const duration = Math.max(Number(client.context.settings.autoSeconds ?? 5) * 1000, scene.beats[beat].text.length * 70);
  autoTimer = setTimeout(() => void moveLine(1), duration);
}

function renderStudio() {
  const pending = turn ? entry(turn)?.value.pendingArt : null;
  renderArt(t, targets(), game().cast, currentAssets(), urls, !!artJob || !!pending || busy, target => generateArt(target), async (key, asset) => {
    try { await writeScene(turn, snapshot.branchId, value => ({ ...value, assets: { ...value.assets, [key]: asset } })); await loadAssets(); render(); }
    catch (error) { report(error); render(); }
  });
  elements.artStatus.textContent = t[artStatus] ?? '';
  if (artJob) {
    const job = artJob;
    if (job.status === 'unresolved') {
      elements.artStatus.textContent = t.artRecover;
      const recover = document.createElement('button'); recover.textContent = t.artRecoverAction;
      recover.onclick = () => void generateArt(job.pending, job.source, job.branchId);
      elements.artStatus.append(' ', recover);
    }
    const cancel = document.createElement('button'); cancel.textContent = t.cancel;
    cancel.onclick = async () => {
      cancel.disabled = true;
      try { await api.cancelImage(job.pending.commandId); if (job.status === 'unresolved') await generateArt(job.pending, job.source, job.branchId); }
      catch (error) { console.error('Illustration cancellation failed:', error.code ?? error.message); }
      finally { cancel.disabled = false; }
    };
    elements.artStatus.append(' ', cancel);
  }
}

async function generateArt(target, source = turn, branchId = snapshot.branchId) {
  if (!source || artJob?.status === 'running' || (artJob && artJob.pending.commandId !== target.commandId)) return;
  const key = `${branchId}:${source.id}:${source.revision}`;
  const retained = records.get(key)?.value.pendingArt;
  if (retained && retained.commandId !== target.commandId) return;
  const pending = target.commandId ? target : { commandId: crypto.randomUUID(), key: target.key, prompt: target.prompt, kind: target.kind };
  if (new TextEncoder().encode(pending.prompt).byteLength > 65536) { artStatus = 'artTooLarge'; render(); return; }
  artJob = { source, branchId, pending, status: 'running' }; artStatus = 'artGenerating'; render();
  try {
    if (target.commandId) {
      // A lost save response may already have committed the final asset binding.
      const fresh = await api.record('scene', source, branchId);
      fresh.value = restoreScene(fresh.value); records.set(key, fresh);
      if (fresh.value.pendingArt?.commandId !== pending.commandId) {
        artStatus = fresh.value.assets[pending.key] ? 'artSaved' : '';
        await loadAssets(); return;
      }
    } else await writeScene(source, branchId, value => ({ ...value, pendingArt: pending }));
    let result = await api.image(pending);
    while (result.status === 'running') { await nextPoll(); result = await api.imageResult(pending.commandId); }
    switch (result.status) {
      case 'completed':
        if (!result.images?.[0]?.asset) throw new Error('Image generation completed without an asset');
        await writeScene(source, branchId, value => ({ ...value, pendingArt: null, assets: { ...value.assets, [pending.key]: result.images[0].asset } }));
        artStatus = 'artSaved'; break;
      case 'cancelled': artStatus = 'artCancelled'; await writeScene(source, branchId, value => ({ ...value, pendingArt: null })); break;
      case 'failed': case 'interrupted': artStatus = result.error?.code === 'NOT_CONFIGURED' ? 'artUnavailable' : 'artFailed'; await writeScene(source, branchId, value => ({ ...value, pendingArt: null })); break;
      default: throw new Error(`Unsupported illustration state: ${result.status}`);
    }
    await loadAssets();
  } catch (error) {
    console.error('Visual novel illustration failed:', error.code ?? error.message);
    artStatus = ['NOT_CONFIGURED', 'MODEL_UNAVAILABLE', 'MODEL_SLOT_UNRESOLVED'].includes(error.code) ? 'artUnavailable' : 'artFailed';
    // Admission errors are definitive; uncertain network failures keep the ID.
    if (['NOT_CONFIGURED', 'INVALID_ARGUMENT', 'PERMISSION_DENIED', 'MODEL_SLOT_UNRESOLVED'].includes(error.code)) {
      try { await writeScene(source, branchId, value => ({ ...value, pendingArt: null })); }
      catch (saveError) { report(saveError); }
    }
    if (records.get(key)?.value.pendingArt) { artJob.status = 'unresolved'; recoverable = true; }
  } finally { if (artJob?.status !== 'unresolved') artJob = null; render(); }
}

function drawCast() { renderCast(t, draftCast, defaultCast(), library, game().configured, toggleCast); }
async function toggleCast(item) {
  if (game().configured) return;
  if (draftCast.some(character => character.id === item.id)) draftCast = draftCast.filter(character => character.id !== item.id);
  else {
    if (draftCast.length >= limits.cast) { elements.castStatus.textContent = t.castLimit; return; }
    try {
      let character = item;
      if (item.libraryId) { const detail = await api.item(item.libraryId); character = { id: item.id, libraryId: item.libraryId, name: detail.name, profile: detail.content, image: detail.image, tagline: detail.briefDescription }; }
      if (character.profile.length > limits.profile) { elements.castStatus.textContent = t.castLimit; return; }
      if (!draftCast.some(candidate => candidate.id === character.id) && draftCast.length < limits.cast) draftCast = [...draftCast, character];
    } catch (error) { console.error('Cast selection failed:', error.code ?? error.message); elements.castStatus.textContent = t.libraryFailed; return; }
  }
  drawCast();
}

async function searchLibrary(append = false) {
  elements.libraryStatus.textContent = t.loading;
  try {
    const result = await api.library(elements.query.value.trim(), append ? libraryOffset : 0);
    const items = result.items.map(item => ({ id: `library:${item.id}`, libraryId: item.id, name: item.name, tagline: item.briefDescription }));
    library = append ? [...library, ...items] : items; libraryOffset = result.nextOffset;
    elements.libraryMore.hidden = libraryOffset === undefined;
    elements.libraryStatus.textContent = library.length ? '' : t.libraryEmpty; drawCast();
  } catch (error) { console.error('Library search failed:', error.code ?? error.message); elements.libraryStatus.textContent = t.libraryFailed; }
}

function drawHistory() {
  renderHistory(t, snapshot.turns, source => { elements.historyDialog.close(); void showTurn(source); }, snapshot.hasMore, async () => {
    try { const page = await api.history(snapshot.branchId, snapshot.beforeCursor); snapshot.turns = [...page.turns, ...snapshot.turns]; snapshot.beforeCursor = page.beforeCursor; snapshot.hasMore = page.hasMore; drawHistory(); }
    catch (error) { report(error); render(); }
  });
}

elements.begin.onclick = () => advance('');
elements.previous.onclick = () => { stopAuto(); void moveLine(-1); };
elements.next.onclick = () => { stopAuto(); void moveLine(1); };
elements.auto.onclick = () => { auto = !auto; scheduleAuto(); render(); };
elements.composer.onsubmit = event => { event.preventDefault(); void advance(elements.input.value); };
elements.input.onkeydown = event => { if (event.key === 'Enter' && !event.shiftKey && !event.isComposing) { event.preventDefault(); elements.composer.requestSubmit(); } };
elements.retry.onclick = () => { if (busy) return; if (snapshot?.status === 'suspended') void performCommand({ kind: 'resume', commandId: crypto.randomUUID(), interruptionId: snapshot.interruptionId, branchId: snapshot.branchId }); else void initialize(); };
elements.cancel.onclick = async () => {
  elements.cancel.disabled = true;
  try { if (snapshot.operationId) snapshot = await api.command({ kind: 'stop', commandId: crypto.randomUUID(), operationId: snapshot.operationId }); }
  catch (error) { report(error); render(); }
  finally { elements.cancel.disabled = false; }
};
elements.castButton.onclick = () => { stopAuto(); draftCast = structuredClone(game().cast); elements.premise.value = game().premise; drawCast(); elements.castDialog.showModal(); void searchLibrary(); };
elements.saveCast.onclick = async () => {
  if (!draftCast.length || draftCast.length > limits.cast) { elements.castStatus.textContent = t.castLimit; return; }
  elements.saveCast.disabled = true;
  try { await writeGame(value => ({ ...value, cast: draftCast, premise: elements.premise.value.trim() })); await loadAssets(); elements.castStatus.textContent = t.castSaved; elements.castDialog.close(); render(); }
  catch (error) { report(error); elements.castStatus.textContent = t.failed; render(); }
  finally { elements.saveCast.disabled = false; }
};
elements.librarySearch.onsubmit = event => { event.preventDefault(); void searchLibrary(); };
elements.libraryMore.onclick = () => void searchLibrary(true);
elements.historyButton.onclick = () => { stopAuto(); drawHistory(); elements.historyDialog.showModal(); };
elements.artButton.onclick = () => { stopAuto(); elements.artDialog.showModal(); renderStudio(); };
document.addEventListener('visibilitychange', () => { if (document.hidden) { stopAuto(); render(); } });
window.addEventListener('denova:appearance', () => { render(); if (elements.castDialog.open) drawCast(); if (elements.historyDialog.open) drawHistory(); });
window.addEventListener('pagehide', () => { for (const url of urls.values()) URL.revokeObjectURL(url); });
await initialize();
