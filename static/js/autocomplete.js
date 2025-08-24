// Autocomplete module: reusable, provider-agnostic, minimal DOM footprint
// Usage:
// import { Autocomplete, makeEndpointFetcher } from './autocomplete.js';
// const ac = new Autocomplete(inputEl, {
//   fetcher: makeEndpointFetcher('/suggest'),
//   onSelect: (text) => { /* fill input, submit form, navigate, etc. */ },
//   container: inputEl.closest('.search-wrap'),
//   maxItems: 8,
//   cacheTTL: 60000,
//   perKeystroke: true,
//   minLength: 1,
// });

// Create a fetcher against a backend endpoint returning { suggestions: [] } JSON.
// You can pass a mapper to adapt different payload shapes into a string[] list.
export function makeEndpointFetcher(endpoint = '/suggest', mapper) {
  return async function fetchSuggestions(q, { signal } = {}) {
    const u = new URL(window.location.origin + endpoint);
    u.searchParams.set('q', q);
    u.searchParams.set('_ts', String(Date.now()));
    const res = await fetch(u.toString(), { cache: 'no-store', signal });
    if (!res.ok) return [];
    const data = await res.json().catch(() => ({}));
    if (typeof mapper === 'function') {
      try { return (mapper(data, q) || []).filter(Boolean); } catch { return []; }
    }
    const list = (data && Array.isArray(data.suggestions)) ? data.suggestions : [];
    return list.filter(Boolean);
  };
}

// Compose multiple fetchers. Modes:
// - strategy: 'race' -> render first non-empty; update as better results arrive
// - strategy: 'merge' -> await all, merge + dedupe up to limit
export function makeCompositeFetcher(fetchers = [], { strategy = 'race', limit = 10 } = {}) {
  const dedupe = (arr) => {
    const seen = new Set();
    const out = [];
    for (const s of arr) {
      const k = String(s).toLowerCase();
      if (!seen.has(k)) { seen.add(k); out.push(String(s)); }
      if (out.length >= limit) break;
    }
    return out;
  };
  if (!Array.isArray(fetchers) || fetchers.length === 0) {
    return async () => [];
  }
  if (strategy === 'merge') {
    return async function merged(q, { signal } = {}) {
      const promises = fetchers.map(f => f(q, { signal }).catch(() => []));
      const lists = await Promise.all(promises);
      return dedupe([].concat(...lists));
    };
  }
  // race: return first non-empty; if all empty, last wins
  return async function raced(q, { signal } = {}) {
    let best = [];
    await Promise.all(fetchers.map(async (f) => {
      try {
        const list = await f(q, { signal });
        if (!best.length && list && list.length) { best = list.slice(0, limit); }
      } catch (_) {}
    }));
    return dedupe(best);
  };
}

export class Autocomplete {
  constructor(inputEl, opts = {}) {
    if (!inputEl) throw new Error('Autocomplete requires an input element');
    this.input = inputEl;
    this.opts = Object.assign({
      fetcher: makeEndpointFetcher('/suggest'),
      onSelect: (text) => { this.input.value = text; },
      container: inputEl.closest('.search-wrap') || inputEl.parentElement || document.body,
      maxItems: 8,
      cacheTTL: 60_000,
      perKeystroke: true,
      minLength: 1,
    }, opts);

    this.box = null;
    this.items = [];
    this.index = -1;
    this.cache = new Map(); // key -> { ts, items }
    this.abort = null;
    this.token = 0;

    this._ensureBox();
    this._bind();
  }

  destroy() {
    if (this.abort) { this.abort.abort(); this.abort = null; }
    if (this.box && this.box.parentNode) this.box.parentNode.removeChild(this.box);
    this.items = [];
  }

  _ensureBox() {
    if (this.box) return;
    const wrap = this.opts.container;
    if (wrap && getComputedStyle(wrap).position === 'static') {
      wrap.style.position = 'relative';
    }
    const box = document.createElement('div');
    box.id = 'suggest-box';
    box.style.position = 'absolute';
    box.style.left = '0';
    box.style.right = '0';
    box.style.top = 'calc(100% + 2px)';
    box.style.width = '100%';
    box.style.border = '1px solid var(--border)';
    box.style.background = 'var(--bg)';
    box.style.zIndex = '20';
    box.style.boxShadow = 'var(--elev)';
    box.style.borderRadius = '0';
    box.style.display = 'none';
    wrap.appendChild(box);
    this.box = box;

    // delegation for clicks
    this.box.addEventListener('click', (e) => {
      const a = e.target.closest('a[data-idx]');
      if (!a) return;
      e.preventDefault();
      const idx = parseInt(a.getAttribute('data-idx'), 10);
      const val = this.items[idx];
      if (val) this._select(val);
    });
  }

  _bind() {
    const input = this.input;
    input.setAttribute('autocomplete', 'off');

    input.addEventListener('input', () => {
      const v = (input.value || '').trim();
      if (!v || v.length < this.opts.minLength) { this.hide(); return; }
      // show prefix-cached immediately
      const prefix = this._bestPrefix(v);
      if (prefix) {
        const filtered = this._filterStartsWith(prefix.items, v).slice(0, this.opts.maxItems);
        if (filtered.length) this.render(filtered);
      }
      if (this.opts.perKeystroke) this._request(v);
    });

    input.addEventListener('keydown', (e) => {
      if (!this.box || this.box.style.display === 'none') return;
      const max = this.items.length || 0;
      if (!max) return;
      if (e.key === 'ArrowDown') { e.preventDefault(); this.index = (this.index + 1) % max; this._highlight(); }
      else if (e.key === 'ArrowUp') { e.preventDefault(); this.index = (this.index - 1 + max) % max; this._highlight(); }
      else if (e.key === 'Enter') {
        if (this.index >= 0 && this.index < max) {
          e.preventDefault(); this._select(this.items[this.index]);
        }
      } else if (e.key === 'Tab') {
        if (this.index >= 0 && this.index < max) {
          // Insert suggestion into input but do NOT submit; let user keep typing
          e.preventDefault();
          const val = this.items[this.index];
          this.input.value = val;
          // Move caret to end
          try { this.input.setSelectionRange(val.length, val.length); } catch (_) {}
          // Refresh suggestions for the new value and keep box visible
          const current = (this.input.value || '').trim();
          if (current) {
            // Reset highlight so further arrows start from top
            this.index = -1;
            this._request(current);
          } else {
            this.hide();
          }
        }
      } else if (e.key === 'Escape') { this.hide(); }
    });

    input.addEventListener('focus', () => {
      const v = (input.value || '').trim();
      if (v && this.items.length) this.show();
    });
    input.addEventListener('blur', () => setTimeout(() => this.hide(), 120));

    document.addEventListener('click', (e) => {
      if (!this.box) return;
      if (!this.box.contains(e.target) && e.target !== input) this.hide();
    });
  }

  _filterStartsWith(items, q) {
    const ql = q.toLowerCase();
    return (items || []).filter(s => (s || '').toLowerCase().startsWith(ql));
  }

  _bestPrefix(q) {
    let best = null;
    let bestLen = -1;
    const now = Date.now();
    for (const [k, v] of this.cache.entries()) {
      if (!k || !v || (now - v.ts) >= this.opts.cacheTTL) continue;
      if (q.startsWith(k) && k.length > bestLen) { best = v; bestLen = k.length; }
    }
    return best;
  }

  async _request(q) {
    // cache exact
    const now = Date.now();
    const cached = this.cache.get(q);
    if (cached && (now - cached.ts) < this.opts.cacheTTL) {
      this.render(cached.items);
      return;
    }
    try {
      if (this.abort) this.abort.abort();
      this.abort = new AbortController();
      const token = ++this.token;
      const items = await this.opts.fetcher(q, { signal: this.abort.signal });
      if (token !== this.token) return; // stale
      this.cache.set(q, { ts: Date.now(), items });
      // prefer startsWith result for current input value
      const current = (this.input.value || '').trim();
      if (!current) { this.hide(); return; }
      if (current !== q) {
        const filtered = this._filterStartsWith(items, current).slice(0, this.opts.maxItems);
        this.render(filtered);
      } else {
        this.render(items.slice(0, this.opts.maxItems));
      }
    } catch (_) {
      // keep current UI
    }
  }

  render(list) {
    this.items = Array.isArray(list) ? list : [];
    if (!this.items.length) { this.hide(); return; }
    this.index = -1;
    this.box.innerHTML = '<ul style="list-style:none;margin:0;padding:0;">' +
      this.items.map((s, i) => `
        <li>
          <a href="#" data-idx="${i}" style="display:block;padding:8px 12px;text-decoration:none;color:inherit;${i>0?'border-top:1px solid #f1f5f9;':''}">
            ${String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;')}
          </a>
        </li>`).join('') +
      '</ul>';
    this.show();
  }

  show() { if (this.box) this.box.style.display = 'block'; }
  hide() {
    this.index = -1;
    this.items = [];
    if (this.box) this.box.style.display = 'none';
    if (this.abort) { this.abort.abort(); this.abort = null; }
  }

  // Helpers for external integration
  isOpen() { return !!(this.box && this.box.style.display !== 'none' && this.items && this.items.length); }
  getIndex() { return this.index; }

  _highlight() {
    if (!this.box) return;
    const links = this.box.querySelectorAll('a[data-idx]');
    links.forEach((a, i) => {
      a.style.background = (i === this.index) ? '#f8fafc' : 'transparent';
      a.style.fontWeight = (i === this.index) ? '600' : '400';
    });
  }

  _select(text) {
    this.opts.onSelect(text);
    this.hide();
  }
}
