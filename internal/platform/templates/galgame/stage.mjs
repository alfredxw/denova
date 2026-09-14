// UI ownership stays in the game: the host has no dialogue, portrait or CG types.
export const elements = Object.fromEntries([...document.querySelectorAll('[id]')].map(element => [element.id, element]));

export function localize(t, context) {
  document.documentElement.lang = context.locale;
  document.documentElement.dataset.theme = context.theme;
  document.documentElement.dataset.textSize = context.settings.textSize ?? 'standard';
  document.title = t.title;
  for (const element of document.querySelectorAll('[data-text]')) element.textContent = t[element.dataset.text];
  elements.input.placeholder = t.inputPlaceholder;
  elements.query.placeholder = t.searchPlaceholder;
}

function node(tag, className, text) {
  const element = document.createElement(tag);
  if (className) element.className = className;
  if (text !== undefined) element.textContent = text;
  return element;
}

function picture(element, url) {
  element.hidden = !url;
  if (url && element.getAttribute('src') !== url) element.src = url;
  if (!url) element.removeAttribute('src');
}

export function renderStage(t, view) {
  const { scene, turn, cast, beat: index, ready, busy, atLatest, assets, urls, statusKey, auto, streaming } = view;
  const beat = scene?.beats[index];
  const character = cast.find(item => item.id === beat?.speaker);
  elements.welcome.hidden = !!turn || streaming;
  if (view.adopting) {
    elements.welcome.querySelector('h2').textContent = t.adoptTitle;
    elements.welcome.querySelector('p').textContent = t.adoptText;
    elements.begin.textContent = t.adoptBegin;
  }
  elements.dialogue.hidden = !beat;
  elements.chapter.textContent = turn ? `${t.chapter} / ${String(view.turnIndex + 1).padStart(2, '0')}` : '';
  elements.sceneTitle.textContent = scene?.title ?? '';
  elements.speaker.textContent = character?.name ?? t.narrator;
  elements.expression.textContent = character ? t.expressions[beat.expression] : '';
  elements.progress.textContent = streaming ? t.live : beat ? `${index + 1} / ${scene.beats.length}` : '';
  elements.line.textContent = beat?.text ?? '';
  elements.status.textContent = t[statusKey] ?? t.failed;
  elements.status.parentElement.dataset.busy = String(busy);
  elements.begin.disabled = !ready || busy;
  elements.previous.disabled = !beat || index === 0 || (busy && !streaming);
  elements.next.disabled = !beat || index >= scene.beats.length - 1 || (busy && !streaming);
  elements.auto.textContent = auto ? t.autoStop : t.auto;
  elements.auto.setAttribute('aria-pressed', String(auto));
  elements.auto.disabled = !beat || index >= scene.beats.length - 1 || busy;
  elements.cancel.hidden = !busy;
  elements.retry.hidden = busy || (ready && !view.recoverable);
  elements.castButton.disabled = !ready || busy;
  elements.historyButton.disabled = !ready || busy;
  elements.artButton.disabled = !ready;
  const end = !!beat && index >= scene.beats.length - 1;
  elements.composer.hidden = !turn || !atLatest || streaming;
  elements.input.disabled = busy || !ready;
  elements.send.disabled = busy || !ready;
  elements.actionHint.textContent = !atLatest && turn ? t.replayHint : end ? t.actionHint : t.readingHint;
  elements.choices.replaceChildren();
  if (end && atLatest && !streaming) for (const [choiceIndex, choice] of (scene?.choices?.length ? scene.choices : turn?.choices ?? []).entries()) {
    const button = node('button', '', '');
    button.append(node('small', '', String(choiceIndex + 1).padStart(2, '0')), document.createTextNode(choice));
    button.disabled = busy || !ready; button.onclick = () => view.onAction(choice); elements.choices.append(button);
  }
  if (turn && !atLatest) { const button = node('button', '', t.latestScene); button.disabled = busy; button.onclick = view.onLatest; elements.choices.append(button); }
  picture(elements.background, urls.get(assetKey(assets[`background:${turn?.id}`])) ?? './art/station.png');
  picture(elements.cg, streaming ? null : urls.get(assetKey(assets[`cg:${turn?.id}:${index}`])));
  const existing = new Map([...elements.portraits.children].map(element => [element.dataset.character, element]));
  const portraits = cast.map(item => {
    const expression = character?.id === item.id ? beat.expression : 'neutral';
    const url = urls.get(assetKey(assets[`portrait:${item.id}:${expression}`] ?? assets[`portrait:${item.id}:neutral`] ?? item.image)) ?? (['lin', 'xu'].includes(item.id) ? `./art/${item.id}.png` : null);
    let portrait = existing.get(item.id);
    if (!portrait) {
      portrait = node('div', 'portrait'); portrait.dataset.character = item.id;
      const image = node('img'); image.alt = item.name;
      const placeholder = node('div', 'portrait-placeholder', item.name.slice(0, 1));
      portrait.append(placeholder, image);
    }
    portrait.classList.toggle('active', !!character && character.id === item.id);
    portrait.querySelector('.portrait-placeholder').hidden = !!url;
    picture(portrait.querySelector('img'), url);
    return portrait;
  });
  elements.portraits.replaceChildren(...portraits);
}

export function assetKey(asset) { return asset ? `${asset.kind}:${asset.path}` : ''; }

export function renderCast(t, selected, defaults, library, locked, onToggle) {
  const selectedIds = new Set(selected.map(item => item.id));
  const card = (item, source) => {
    const element = node('article', 'cast-card'), content = node('div');
    content.append(node('h3', '', item.name), node('p', '', item.tagline || source));
    const button = node('button', '', selectedIds.has(item.id) ? t.castRemove : t.castAdd);
    button.disabled = locked; button.setAttribute('aria-pressed', String(selectedIds.has(item.id))); button.onclick = () => onToggle(item);
    element.append(node('span', 'cast-icon', item.name.slice(0, 1)), content, button); return element;
  };
  const all = [...defaults, ...selected.filter(item => !defaults.some(candidate => candidate.id === item.id))];
  elements.castList.replaceChildren(...all.map(item => card(item, item.libraryId ? t.castLibrary : t.castDefault)));
  elements.libraryList.replaceChildren(...library.map(item => card(item, t.castLibrary)));
  elements.premise.disabled = locked;
  elements.saveCast.disabled = locked;
  elements.castStatus.textContent = locked ? t.castLocked : '';
}

export function renderHistory(t, turns, onRead, hasMore, onMore) {
  elements.historyList.replaceChildren();
  if (!turns.length) elements.historyList.append(node('p', 'dialog-intro', t.historyEmpty));
  for (const [index, turn] of turns.entries()) {
    const card = node('article', 'history-card'), button = node('button', '', t.readScene);
    button.onclick = () => onRead(turn);
    card.append(node('small', '', `${t.chapter} ${index + 1}`), node('p', '', turn.narrative), button); elements.historyList.append(card);
  }
  if (hasMore) { const button = node('button', '', t.more); button.onclick = onMore; elements.historyList.prepend(button); }
}

export function renderArt(t, targets, cast, assets, urls, busy, onGenerate, onLibrary) {
  elements.artList.replaceChildren();
  if (!targets.length) elements.artList.append(node('p', 'dialog-intro', t.artEmpty));
  for (const target of targets) {
    const character = cast.find(item => item.id === target.character);
    const label = target.kind === 'portrait' ? `${character?.name ?? t.artPortrait} · ${t.expressions[target.expression]}` : target.kind === 'background' ? t.artBackground : t.artCG;
    const card = node('article', 'art-card'), content = node('div', 'art-card-content');
    const url = urls.get(assetKey(assets[target.key]));
    if (url) { const img = node('img'); img.src = url; img.alt = label; card.append(img); }
    const button = node('button', '', assets[target.key] ? t.artRegenerate : t.artGenerate);
    button.disabled = busy; button.onclick = () => onGenerate(target);
    content.append(node('h3', '', label), button);
    if (target.kind === 'portrait' && character?.image) {
      const use = node('button', '', t.artUseLibrary); use.disabled = busy; use.onclick = () => onLibrary(target.key, character.image); content.append(use);
    }
    card.append(content); elements.artList.append(card);
  }
}

for (const button of document.querySelectorAll('[data-close]')) button.onclick = () => elements[button.dataset.close].close();
