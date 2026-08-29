import { initTheme } from './theme.js';

const KEY = 'searxgo.enabledEngines';

function $(sel, root=document) { return root.querySelector(sel); }
function el(tag, props={}) { const e = document.createElement(tag); Object.assign(e, props); return e; }

function loadEnabled() {
  try {
    const raw = localStorage.getItem(KEY);
    if (!raw) return null;
    const arr = JSON.parse(raw);
    return Array.isArray(arr) ? arr : null;
  } catch { return null; }
}

function saveEnabled(arr) {
  try { localStorage.setItem(KEY, JSON.stringify(arr || [])); } catch {}
}

async function fetchEngines() {
  try {
    const u = new URL(window.location.origin + '/engines');
    u.searchParams.set('_ts', String(Date.now()));
    const res = await fetch(u.toString(), { cache: 'no-store' });
    if (!res.ok) return [];
    const data = await res.json();
    const list = (data && Array.isArray(data.engines)) ? data.engines : [];
    return list.filter(Boolean);
  } catch { return []; }
}

function render(list) {
  const wrap = $('#engines-list');
  if (!wrap) return;
  wrap.innerHTML = '';
  const enabled = loadEnabled();
  const selected = new Set((enabled && enabled.length ? enabled : list).map(s => String(s).toLowerCase()));

  list.forEach(name => {
    const li = el('li', { className: 'setting-item' });
    const id = `eng-${name}`;

    // Left: name
    const nameSpan = el('span', { className: 'setting-name', textContent: name });

    // Right: switch
    const switchLabel = el('label', { className: 'switch', htmlFor: id });
    const cb = el('input', { type: 'checkbox', id });
    cb.setAttribute('aria-label', `Enable ${name}`);
    cb.checked = selected.has(String(name).toLowerCase());
    cb.dataset.engine = name;
    const slider = el('span', { className: 'switch-slider', ariaHidden: 'true' });
    switchLabel.appendChild(cb);
    switchLabel.appendChild(slider);

    li.appendChild(nameSpan);
    li.appendChild(switchLabel);
    wrap.appendChild(li);
  });

  const saveBtn = $('#save-prefs');
  const status = $('#save-status');
  saveBtn?.addEventListener('click', () => {
    const next = [];
    wrap.querySelectorAll('input[type="checkbox"]').forEach(n => {
      if (n.checked) next.push(n.dataset.engine);
    });
    saveEnabled(next);
    if (status) {
      status.textContent = 'Saved';
      setTimeout(() => { status.textContent = ''; }, 1500);
    }
  });
}

document.addEventListener('DOMContentLoaded', async () => {
  initTheme();
  const engines = await fetchEngines();
  render(engines);
});
