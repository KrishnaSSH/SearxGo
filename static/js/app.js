import { $ } from './utils.js';
import { renderTimings } from './timings.js';
import { renderKnowledge } from './knowledge.js';
import { renderResults } from './resultsList.js';
import { redditCardState, renderRedditCard } from './redditCard.js';
import { Autocomplete, makeEndpointFetcher } from './autocomplete.js';
import { initTheme } from './theme.js';

document.addEventListener('DOMContentLoaded', () => {
  initTheme();
  const resultsEl = $('#results');
  const timingBar = $('#timing-bar');
  const cardsEl = $('#cards');
  const pagerEl = document.querySelector('.pager');
  const knowledgeEl = $('#knowledge');
  const form = document.querySelector('form.search');
  const input = $('#q');

  const url = new URL(window.location.href);
  const params = url.searchParams;
  let q = (params.get('q') || '').trim();
  let page = parseInt(params.get('page') || '1', 10);
  if (!(page > 0)) page = 1;
  const pageSize = 40;

  if (input) input.value = q;

  // AbortController to cancel in-flight knowledge requests when a new search starts
  let knowledgeAbort = null;

  // Initialize modular autocomplete on both index and results pages
  let ac = null;
  if (input) {
    ac = new Autocomplete(input, {
      fetcher: makeEndpointFetcher('/suggest'),
      container: input.closest('.search-wrap') || input.parentElement,
      maxItems: 8,
      cacheTTL: 60_000,
      perKeystroke: true,
      minLength: 1,
      onSelect: (text) => {
        input.value = text;
        if (resultsEl) {
          q = text;
          updateURLAndState(1);
          fetchAndRender();
        } else if (form) {
          // On home, navigate directly to results to avoid any submit quirks
          const target = new URL(window.location.origin + '/results');
          target.searchParams.set('q', text);
          window.location.href = target.toString();
        }
      }
    });
  }

  // Preferences: enabled engines helpers
  function getEnabledEngines() {
    try {
      const raw = localStorage.getItem('searxgo.enabledEngines');
      if (!raw) return null;
      const arr = JSON.parse(raw);
      if (Array.isArray(arr)) return arr.map(s => String(s).toLowerCase());
    } catch (_) {}
    return null;
  }
  function isEngineEnabled(name) {
    const list = getEnabledEngines();
    if (!list || list.length === 0) return true; // default: all enabled if unset
    return list.includes(String(name).toLowerCase());
  }

  function updateURLAndState(newPage) {
    page = newPage;
    const u = new URL(window.location.href);
    u.searchParams.set('q', q);
    if (page === 1) {
      u.searchParams.delete('page');
    } else {
      u.searchParams.set('page', String(page));
    }
    history.pushState({ q, page }, '', u);
  }

  function updatePager() {
    if (!pagerEl) return;
    const items = [];
    // Prev
    items.push(`<a href="#" class="pager-btn${page <= 1 ? ' is-disabled' : ''}" data-action="prev" aria-label="Previous page" aria-disabled="${page <= 1}">Previous</a>`);
    // First page always
    items.push(`<a href="#" class="pager-page${page === 1 ? ' is-active' : ''}" data-page="1" aria-label="Page 1">1</a>`);
    // Determine window around current page
    const start = Math.max(2, page - 2);
    const end = Math.max(start, page + 2);
    if (start > 2) {
      items.push(`<span class="pager-ellipsis" aria-hidden="true">…</span>`);
    }
    for (let p = start; p <= end; p++) {
      // Avoid duplicating page 1
      if (p === 1) continue;
      items.push(`<a href="#" class="pager-page${page === p ? ' is-active' : ''}" data-page="${p}" aria-label="Page ${p}">${p}</a>`);
    }
    // Trailing ellipsis to indicate more pages
    items.push(`<span class="pager-ellipsis" aria-hidden="true">…</span>`);
    // Next
    items.push(`<a href="#" class="pager-btn" data-action="next" aria-label="Next page">Next</a>`);
    pagerEl.innerHTML = items.join('');
  }

  async function fetchAndRender() {
    if (!resultsEl) return; // don't run on index page
    if (!q) { resultsEl.innerHTML = '<p>Enter a query above.</p>'; return; }
    resultsEl.innerHTML = '<p>Searching…</p>';
    if (knowledgeEl) knowledgeEl.innerHTML = '';

    // Clear Reddit card immediately for new queries; it will render after new results arrive
    if (cardsEl) { cardsEl.innerHTML = ''; }

    // Knowledge fetch in parallel with progressive retry to ensure logo+facts
    let knowledgePromise = null;
    if (knowledgeEl) {
      // cancel any previous knowledge fetch to avoid stale overwrites
      if (knowledgeAbort) {
        try { knowledgeAbort.abort(); } catch (_) {}
      }
      knowledgeAbort = new AbortController();
      const { signal } = knowledgeAbort;
      // Keep track of last best card to avoid regressions (facts disappearing, logo lost)
      let lastCard = null;
      const normalize = (c) => {
        if (!c) return null;
        // Accept both server and client-case fields
        const n = {
          Title: c.Title || c.title || '',
          Thumbnail: c.Thumbnail || c.thumbnail || '',
          Facts: Array.isArray(c.Facts) ? c.Facts : (Array.isArray(c.facts) ? c.facts : []),
          Extract: c.Extract || c.extract || '',
          Description: c.Description || c.description || '',
          URL: c.URL || c.url || '',
          Website: c.Website || c.website || ''
        };
        return n;
      };
      const hasLogo = (c) => !!(c && c.Thumbnail);
      const factsCount = (c) => (c && Array.isArray(c.Facts)) ? c.Facts.length : 0;
      const hasExtract = (c) => !!(c && (c.Extract || c.Description));
      const isValid = (c) => !!(c && (c.Title));
      const isBetter = (next, prev) => {
        if (!isValid(next)) return false;
        if (!prev) return true;
        // Do not swap thumbnails once one is set to avoid image jumping
        if (hasLogo(prev)) {
          // If we already have a thumbnail, do not consider image a reason to update
        } else if (hasLogo(next)) {
          return true;
        }
        const nf = factsCount(next), pf = factsCount(prev);
        if (nf > pf) return true;
        if (hasExtract(next) && !hasExtract(prev)) return true;
        return false;
      };

      const fetchKnowledgeOnce = async () => {
        const ku = new URL(window.location.origin + '/knowledge');
        ku.searchParams.set('q', q);
        ku.searchParams.set('_ts', String(Date.now()));
        try {
          const r = await fetch(ku.toString(), { cache: 'no-store', signal });
          if (!r.ok) return null;
          return await r.json();
        } catch (_) { return null; }
      };

      const hasLogoAndFacts = (card) => {
        const c = normalize(card);
        return !!(c && c.Thumbnail && Array.isArray(c.Facts) && c.Facts.length);
      };

      const fetchWithRetry = async (attempts = 2, delayMs = 900) => {
        let data = await fetchKnowledgeOnce();
        const n0 = normalize(data);
        // If first response lacks thumbnail, hold initial render briefly to reduce flicker
        if (n0 && !n0.Thumbnail) {
          // wait for one retry before rendering to allow image/facts
        } else if (isBetter(n0, normalize(lastCard))) {
          // preserve existing thumbnail to avoid swapping images
          if (lastCard) {
            const prev = normalize(lastCard);
            if (prev && prev.Thumbnail) {
              try {
                const merged = { ...data };
                merged.Thumbnail = prev.Thumbnail;
                merged.thumbnail = prev.Thumbnail;
                data = merged;
              } catch (_) {}
            }
          }
          lastCard = data;
          renderKnowledge(knowledgeEl, data);
        }
        if (hasLogoAndFacts(data)) return;
        for (let i = 0; i < attempts; i++) {
          await new Promise(res => setTimeout(res, delayMs));
          data = await fetchKnowledgeOnce();
          if (isBetter(normalize(data), normalize(lastCard))) {
            // preserve existing thumbnail to avoid swapping images
            if (lastCard) {
              const prev = normalize(lastCard);
              if (prev && prev.Thumbnail) {
                try {
                  const merged = { ...data };
                  merged.Thumbnail = prev.Thumbnail;
                  merged.thumbnail = prev.Thumbnail;
                  data = merged;
                } catch (_) {}
              }
            }
            lastCard = data;
            renderKnowledge(knowledgeEl, data);
          }
          if (hasLogoAndFacts(data)) break;
        }
        // If nothing rendered yet, render the best we have at the end
        if (!lastCard && data) {
          lastCard = data;
          renderKnowledge(knowledgeEl, data);
        }
      };

      knowledgePromise = fetchWithRetry(2, 1000);
    }

    try {
      const u = new URL(window.location.origin + '/search');
      u.searchParams.set('q', q);
      u.searchParams.set('page', String(page));
      u.searchParams.set('size', String(pageSize));
      u.searchParams.set('timings', '1');
      // append enabled engines from preferences (local-only)
      try {
        const raw = localStorage.getItem('searxgo.enabledEngines');
        if (raw) {
          const enabled = JSON.parse(raw);
          if (Array.isArray(enabled) && enabled.length) {
            u.searchParams.set('engines', enabled.join(','));
          }
        }
      } catch (_) {}
      const res = await fetch(u.toString(), { cache: 'no-store' });
      if (!res.ok) throw new Error('HTTP ' + res.status);
      const data = await res.json();
      const items = Array.isArray(data) ? data : (data.results || []);

      // Render Reddit card only if Reddit engine is enabled, and exclude Reddit items from the main list
      if (cardsEl) {
        if (isEngineEnabled('reddit')) {
          renderRedditCard(cardsEl, items, q, redditCardState);
        } else {
          cardsEl.innerHTML = '';
        }
      }
      const nonReddit = items.filter(r => (r.Engine || r.engine || '').toLowerCase() !== 'reddit');
      renderResults(resultsEl, nonReddit);
      renderTimings(timingBar, data.timings || null, data.took_ms);
      updatePager();
      window.scrollTo({ top: 0, behavior: 'smooth' });
      if (knowledgePromise) knowledgePromise.catch(() => {});
    } catch (err) {
      console.error('fetch error', err);
      resultsEl.innerHTML = '<p>Search failed.</p>';
      renderTimings(timingBar, null, null);
    }
  }

  // redditCard.js attaches its own delegated handlers on first call

  if (form) {
    form.addEventListener('submit', (e) => {
      const v = (input.value || '').trim();
      if (!v) return;
      if (ac) ac.hide();
      if (resultsEl) {
        e.preventDefault();
        q = v;
        updateURLAndState(1);
        fetchAndRender();
      } // else allow normal navigation to /results
    });

    // On home page, ensure Enter always submits even if autocomplete is open with no selection
    if (!resultsEl && input) {
      input.addEventListener('keydown', (e) => {
        if (e.key === 'Enter') {
          const hasSel = !!(ac && ac.isOpen && ac.isOpen() && typeof ac.getIndex === 'function' && ac.getIndex() >= 0);
          if (!hasSel) {
            const v = (input.value || '').trim();
            if (v) {
              e.preventDefault();
              const target = new URL(window.location.origin + '/results');
              target.searchParams.set('q', v);
              window.location.href = target.toString();
            }
          }
        }
      });
    }
  }
  if (pagerEl) {
    pagerEl.addEventListener('click', (e) => {
      const a = e.target.closest('a');
      if (!a) return;
      e.preventDefault();
      const action = a.getAttribute('data-action');
      const targetPageAttr = a.getAttribute('data-page');
      if (action === 'prev') {
        if (page <= 1) return;
        updateURLAndState(page - 1);
        fetchAndRender();
        return;
      }
      if (action === 'next') {
        updateURLAndState(page + 1);
        fetchAndRender();
        return;
      }
      if (targetPageAttr) {
        const target = parseInt(targetPageAttr, 10);
        if (!Number.isNaN(target) && target > 0 && target !== page) {
          updateURLAndState(target);
          fetchAndRender();
        }
      }
    });
  }

  window.addEventListener('popstate', (ev) => {
    const state = ev.state || {};
    if (state.q != null) q = state.q;
    if (state.page != null) page = state.page;
    if (input) input.value = q;
    fetchAndRender();
  });

  // Initial load only on results page
  if (resultsEl) fetchAndRender();
});
