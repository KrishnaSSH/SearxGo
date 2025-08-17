import { $ } from './utils.js';
import { renderTimings } from './timings.js';
import { renderKnowledge } from './knowledge.js';
import { renderResults } from './resultsList.js';
import { redditCardState, renderRedditCard, cacheKey as redditCacheKey, serializeState as redditSerialize, restoreState as redditRestore, buildRedditCardHTML } from './redditCard.js';

document.addEventListener('DOMContentLoaded', () => {
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
  const pageSize = 30;

  if (input) input.value = q;

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
    if (!q) { resultsEl.innerHTML = '<p>Enter a query above.</p>'; return; }
    resultsEl.innerHTML = '<p>Searching…</p>';
    if (knowledgeEl) knowledgeEl.innerHTML = '';

    // Show cached Reddit card (if any) immediately with loading state
    if (cardsEl) {
      const cached = sessionStorage.getItem(redditCacheKey(q));
      if (cached) {
        try {
          const stateObj = JSON.parse(cached);
          redditRestore(stateObj, redditCardState);
          cardsEl.innerHTML = buildRedditCardHTML(redditCardState);
          const card = cardsEl.querySelector('.reddit-card');
          if (card) card.classList.add('is-loading');
        } catch (_) { /* ignore bad cache */ }
      } else {
        cardsEl.innerHTML = '';
      }
    }

    // Knowledge fetch in parallel
    let knowledgePromise = null;
    if (knowledgeEl) {
      const ku = new URL(window.location.origin + '/knowledge');
      ku.searchParams.set('q', q);
      ku.searchParams.set('_ts', String(Date.now()));
      knowledgePromise = fetch(ku.toString(), { cache: 'no-store' })
        .then(r => r.ok ? r.json() : null)
        .then(data => renderKnowledge(knowledgeEl, data))
        .catch(() => {});
    }

    try {
      const u = new URL(window.location.origin + '/search');
      u.searchParams.set('q', q);
      u.searchParams.set('page', String(page));
      u.searchParams.set('size', String(pageSize));
      u.searchParams.set('timings', '1');
      const res = await fetch(u.toString(), { cache: 'no-store' });
      if (!res.ok) throw new Error('HTTP ' + res.status);
      const data = await res.json();
      const items = Array.isArray(data) ? data : (data.results || []);

      // Render Reddit card and exclude Reddit items from the main list
      if (cardsEl) {
        renderRedditCard(cardsEl, items, q, redditCardState);
        try {
          const toCache = redditSerialize({ ...redditCardState, query: q });
          sessionStorage.setItem(redditCacheKey(q), JSON.stringify(toCache));
        } catch (_) {}
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
      e.preventDefault();
      const v = (input.value || '').trim();
      if (!v) return;
      q = v;
      updateURLAndState(1);
      fetchAndRender();
    });
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

  // Initial load
  fetchAndRender();
});
