import { connect } from './client.mjs';
import { followRun } from './stream.mjs';

// Each contact owns one platform session. History lives in its canonical journal.
const characters = [
  { id: 'lamplighter', definition: 'local:character', avatar: '☾' },
  { id: 'archivist', definition: 'local:archivist', avatar: '⌘' },
];
const translations = Object.fromEntries(await Promise.all(['zh-CN', 'en-US'].map(async locale => [locale, await (await fetch(`./locales/${locale}.json`)).json()])));
const client = await connect();
const elements = Object.fromEntries(['eyebrow', 'title', 'intro', 'contacts', 'avatar', 'name', 'bio', 'older', 'messages', 'status', 'retry', 'composer', 'label', 'prompt', 'hint', 'cancel', 'send', 'scene', 'evidence', 'objective', 'resolve', 'caseTitle', 'sceneTitle', 'evidenceTitle'].map(id => [id, document.getElementById(id)]));
let t, selected = 'lamplighter', revision = null, save = { version: 1, pending: null };
let busy = true, ready = false, activeRun = null, partial = '', statusKey = 'loading';
const clues = ['clock', 'notice', 'letter'];
const sessions = new Map(), histories = new Map(), drafts = new Map();

function render() {
  t = translations[client.context.locale];
  document.documentElement.lang = client.context.locale;
  document.documentElement.dataset.theme = client.context.theme;
  document.documentElement.dataset.textSize = client.context.settings.textSize ?? 'standard';
  elements.hint.hidden = client.context.settings.showHints === false;
  document.title = t.title;
  for (const id of ['eyebrow', 'title', 'intro', 'label', 'hint', 'send', 'cancel', 'older', 'retry', 'caseTitle', 'sceneTitle', 'evidenceTitle']) elements[id].textContent = t[id];
  elements.objective.textContent = save.resolved ? t.closed : t.objective;
  elements.resolve.textContent = t.resolve;
  elements.resolve.disabled = !ready || busy || !!save.pending || (save.clues ?? []).length !== clues.length || !!save.resolved;
  for (const id of clues) {
    const button = document.getElementById(id);
    button.textContent = t.clues[id].name;
    button.setAttribute('aria-label', t.clues[id].name);
    button.disabled = !ready || busy || !!save.pending || (id === 'letter' && !save.clues?.includes('clock'));
    button.setAttribute('aria-pressed', String((save.clues ?? []).includes(id)));
  }
  elements.evidence.replaceChildren(...(save.clues ?? []).map(id => {
    const item = document.createElement('li'), name = document.createElement('strong'), detail = document.createElement('p');
    name.textContent = t.clues[id].name; detail.textContent = t.clues[id].detail; item.append(name, detail); return item;
  }));
  if (!save.clues?.length) { const item = document.createElement('li'); item.textContent = t.noEvidence; elements.evidence.append(item); }
  elements.contacts.setAttribute('aria-label', t.contacts);
  elements.messages.setAttribute('aria-label', t.history);
  elements.prompt.placeholder = t.placeholder;
  elements.contacts.replaceChildren(...characters.map(character => {
    const button = document.createElement('button');
    button.className = 'contact'; button.setAttribute('aria-pressed', String(character.id === selected));
    const icon = document.createElement('span'); icon.className = 'avatar'; icon.textContent = character.avatar; icon.setAttribute('aria-hidden', 'true');
    const content = document.createElement('span'), name = document.createElement('strong'), bio = document.createElement('small');
    name.textContent = t[character.id].name; bio.textContent = t[character.id].tagline;
    content.append(name, bio); button.append(icon, content);
    button.onclick = () => { drafts.set(selected, elements.prompt.value); selected = character.id; elements.prompt.value = drafts.get(selected) ?? ''; render(); };
    return button;
  }));
  elements.avatar.textContent = characters.find(character => character.id === selected).avatar;
  elements.name.textContent = t[selected].name; elements.bio.textContent = t[selected].bio;
  elements.status.textContent = t[statusKey];
  elements.prompt.disabled = !ready || busy || !!save.pending;
  elements.send.disabled = elements.prompt.disabled;
  elements.cancel.hidden = !activeRun;
  elements.retry.hidden = busy || (ready && !save.pending);
  elements.older.hidden = !histories.get(selected)?.cursor;
  elements.older.disabled = busy;
  renderMessages();
}

function renderMessages() {
  const timeline = elements.messages.parentElement;
  const follow = timeline.scrollHeight - timeline.scrollTop - timeline.clientHeight < 80;
  const items = histories.get(selected)?.items ?? [];
  elements.messages.replaceChildren();
  if (!items.length && save.pending?.character !== selected) {
    const empty = document.createElement('p'); empty.className = 'empty'; empty.textContent = t[selected].opening; elements.messages.append(empty);
  }
  const append = (role, text) => {
    if (role !== 'user' && role !== 'assistant') return;
    const bubble = document.createElement('article'); bubble.className = `message${role === 'user' ? ' player' : ''}`;
    const name = document.createElement('small'), body = document.createElement('p');
    name.textContent = role === 'user' ? t.you : t[selected].name; body.textContent = text;
    bubble.append(name, body); elements.messages.append(bubble);
  };
  for (const item of items) append(item.role, item.text);
  if (save.pending?.character === selected) {
    if (!items.length || items.at(-1).role !== 'user' || items.at(-1).text !== save.pending.text) append('user', save.pending.text);
    if (partial) append('assistant', partial);
  }
  if (follow) timeline.scrollTop = timeline.scrollHeight;
}

async function persist(next) {
  const result = await client.request('/game-data/file', { method: 'PUT', body: JSON.stringify({ path: 'messages.json', content: JSON.stringify(next), expectedRevision: revision }) });
  revision = result.revision; save = next;
}

async function history(character, older = false) {
  const previous = histories.get(character);
  const page = await client.request(`/agents/sessions/${sessions.get(character)}/history?limit=50&cursor=${encodeURIComponent(older ? previous.cursor : '')}`);
  histories.set(character, { items: older ? [...page.items, ...previous.items] : page.items, cursor: page.cursor });
}

function report(error) {
  console.error('Character chat operation failed:', error.code ?? error.message);
  statusKey = error.code === 'DOCUMENT_CONFLICT' ? 'conflict' : 'failed';
}

async function converse() {
  busy = true; statusKey = 'waiting'; render();
  try {
    const pending = save.pending;
    // Save exact input before POST. Reusing this ID after a lost response finds
    // the durable receipt instead of repeating a completed model call.
    let result = await client.request(`/agents/sessions/${sessions.get(pending.character)}/runs`, { method: 'POST', body: JSON.stringify({ commandId: pending.commandId, input: { text: pending.text } }) });
    activeRun = result.run.runId; render();
    if (['accepted', 'running', 'waiting'].includes(result.status))
      result = await followRun(client, activeRun, text => { partial = text; renderMessages(); });
    switch (result.status) {
      case 'completed': statusKey = 'saved'; break;
      case 'failed': statusKey = 'runFailed'; break;
      case 'aborted': statusKey = 'aborted'; break;
      case 'incomplete': statusKey = 'incomplete'; break;
      default: throw new Error(`Unexpected run status: ${result.status}`);
    }
    await history(pending.character);
    await persist({ ...save, pending: null });
    partial = '';
  } catch (error) { report(error); }
  finally { busy = false; activeRun = null; render(); }
}

async function initialize() {
  busy = true; ready = false; statusKey = 'loading'; render();
  try {
    try {
      const file = await client.request('/game-data/file?path=messages.json');
      const restored = JSON.parse(file.content);
      if (restored.version !== 1 || (restored.pending && (!characters.some(character => character.id === restored.pending.character) || typeof restored.pending.text !== 'string' || typeof restored.pending.commandId !== 'string'))) throw new Error('Unsupported chat save');
      if (restored.clues !== undefined && (!Array.isArray(restored.clues) || restored.clues.some(id => !clues.includes(id)) || new Set(restored.clues).size !== restored.clues.length)) throw new Error('Unsupported evidence save');
      if (restored.resolved !== undefined && (typeof restored.resolved !== 'boolean' || (restored.resolved && restored.clues?.length !== clues.length))) throw new Error('Unsupported case save');
      save = restored; revision = file.revision;
    } catch (error) { if (error.code !== 'NOT_FOUND') throw error; }
    for (const character of characters) {
      const session = await client.request('/agents/sessions', { method: 'POST', body: JSON.stringify({ projectId: client.context.scope.projectId, definition: character.definition, key: `messages-v1:${character.id}` }) });
      sessions.set(character.id, session.ref.sessionId);
      await history(character.id);
    }
    ready = true; statusKey = 'ready';
    if (save.pending) { selected = save.pending.character; await converse(); }
  } catch (error) { report(error); }
  finally { busy = false; render(); }
}

elements.send.onclick = async () => {
  const text = elements.prompt.value.trim();
  if (!text || busy || !ready || save.pending) return;
  const character = selected;
  busy = true; render();
  try {
    await persist({ ...save, pending: { character, commandId: crypto.randomUUID(), text } });
    drafts.delete(character); if (selected === character) elements.prompt.value = '';
    partial = ''; await converse();
  } catch (error) { report(error); ready = false; busy = false; render(); }
};
elements.prompt.onkeydown = event => {
  if (event.key === 'Enter' && !event.shiftKey && !event.isComposing) { event.preventDefault(); elements.send.click(); }
};
elements.cancel.onclick = async () => {
  if (!activeRun) return;
  elements.cancel.disabled = true;
  try { await client.request(`/agents/runs/${activeRun}/cancel`, { method: 'POST' }); }
  catch (error) { report(error); render(); }
  finally { elements.cancel.disabled = false; }
};
elements.retry.onclick = () => { if (!busy) void initialize(); };
elements.older.onclick = async () => {
  busy = true; render();
  try { await history(selected, true); }
  catch (error) { report(error); }
  finally { busy = false; render(); }
};
window.addEventListener('denova:appearance', render);
// Scene rules are deterministic. Model replies cannot grant evidence or close a case.
for (const id of clues) document.getElementById(id).onclick = async () => {
  if (!ready || busy || save.pending || save.clues?.includes(id) || (id === 'letter' && !save.clues?.includes('clock'))) return;
  busy = true; render();
  try { await persist({ ...save, clues: [...(save.clues ?? []), id] }); statusKey = 'discovered'; }
  catch (error) { report(error); ready = false; }
  finally { busy = false; render(); }
};
elements.resolve.onclick = async () => {
  if (!ready || busy || save.pending || save.clues?.length !== clues.length || save.resolved) return;
  busy = true; render();
  try { await persist({ ...save, resolved: true }); statusKey = 'caseSaved'; }
  catch (error) { report(error); ready = false; }
  finally { busy = false; render(); }
};
await initialize();
