import { connect } from './client.mjs';
import english from './locales/en-US.json' with { type: 'json' };
import chinese from './locales/zh-CN.json' with { type: 'json' };

const client = await connect();
const root = document.querySelector('#game-root');
const start = document.querySelector('#start');
let started = false;

// This shell deliberately has no characters, rules, model slots or save data.
// Add your game's UI here and declare only the platform capabilities it uses.
function render() {
  const locale = client.context.locale === 'zh-CN' ? 'zh-CN' : 'en-US';
  const strings = locale === 'zh-CN' ? chinese : english;
  document.documentElement.lang = locale;
  document.documentElement.dataset.theme = client.context.theme;
  document.title = strings.title;
  document.querySelector('#title').textContent = strings.title;
  document.querySelector('#description').textContent = strings.description;
  document.querySelector('#status').textContent = started ? strings.started : strings.ready;
  start.textContent = strings.start;
}

start.addEventListener('click', () => { started = true; render(); });
window.addEventListener('denova:appearance', render);
render();
root.setAttribute('aria-busy', 'false');
start.disabled = false;
