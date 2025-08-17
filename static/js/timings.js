import { escapeHtml, formatMs, prettyEngineName } from './utils.js';

export function renderTimings(container, timings, took) {
  if (!container) return;
  if (!timings && took == null) { container.innerHTML = ''; return; }
  const chips = [];
  if (typeof took === 'number') chips.push(`<span class="timing-total">Total: ${formatMs(took)}</span>`);
  if (Array.isArray(timings)) {
    for (const t of timings) {
      const name = prettyEngineName(t.engine || t.Engine);
      const ms = t.ms ?? t.Ms;
      chips.push(`<span class="timing-chip"><span class="engine">${escapeHtml(name)}</span><span class="ms">${formatMs(ms)}</span></span>`);
    }
  }
  container.innerHTML = chips.join(' ');
}
