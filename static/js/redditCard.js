import { escapeHtml } from './utils.js';

const REDDIT_ICON = 'https://www.redditstatic.com/desktop2x/img/favicon/favicon-32x32.png';

export const redditCardState = { posts: [], topSubs: [], activeSub: 'All', query: '' };

export function buildRedditCardHTML(state) {
  const chips = ['All', ...state.topSubs];
  const chipsHTML = chips.map(s => {
    const isActive = (s === state.activeSub);
    return `<a class="chip${isActive ? ' is-active' : ''}" href="#" data-sub="${escapeHtml(s)}">${s === 'All' ? 'All' : 'r/' + escapeHtml(s)}</a>`;
  }).join('');

  const filtered = state.activeSub === 'All'
    ? state.posts
    : state.posts.filter(p => (p.subreddit || '').toLowerCase() === state.activeSub.toLowerCase());

  const listHTML = filtered.slice(0, 3).map(p => {
    const meta = p.subreddit ? `r/${escapeHtml(p.subreddit)} • Reddit` : 'Reddit';
    return `
      <li class="rc-item">
        <h4 class="rc-title"><a href="${p.url}" target="_blank" rel="noopener">${escapeHtml(p.title)}</a></h4>
        <div class="rc-meta">${meta}</div>
      </li>`;
  }).join('');

  const searchLink = `https://www.reddit.com/search/?q=${encodeURIComponent(state.query || '')}`;

  return `
    <section class="reddit-card card">
      <header class="rc-header">
        <img class="rc-logo" src="${REDDIT_ICON}" alt="Reddit" />
        <h3 class="rc-brand">Reddit</h3>
        <nav class="rc-tabs">${chipsHTML}</nav>
      </header>
      <ul class="rc-list">${listHTML}</ul>
      <div class="rc-actions">
        <a class="rc-more" href="${searchLink}" target="_blank" rel="noopener">Show More ▾</a>
      </div>
    </section>`;
}

export function attachRedditCardHandlers(container, state = redditCardState) {
  if (!container) return;
  if (container.__redditHandlerAttached) return;
  container.__redditHandlerAttached = true;
  container.addEventListener('click', (ev) => {
    const a = ev.target.closest('.reddit-card .chip');
    if (!a) return;
    ev.preventDefault();
    const sub = a.getAttribute('data-sub') || 'All';
    state.activeSub = sub;
    container.innerHTML = buildRedditCardHTML(state);
  });
}

export function renderRedditCard(container, items, query, state = redditCardState) {
  if (!container) return;
  const reddits = (items || []).filter(r => {
    const n = (r.Engine || r.engine || '').toLowerCase();
    return n === 'reddit';
  });
  if (reddits.length === 0) { return; }

  const subs = new Map();
  const posts = [];
  for (const r of reddits) {
    const urlStr = r.URL || r.url || '';
    let sub = '';
    try {
      const u = new URL(urlStr);
      const m = u.pathname.match(/\/r\/([^\/]+)\//);
      if (m) sub = m[1];
    } catch (_) {}
    if (sub) subs.set(sub, (subs.get(sub) || 0) + 1);
    posts.push({ title: r.Title || r.title || r.URL || r.url || '', url: urlStr, subreddit: sub });
  }

  const topSubs = Array.from(subs.entries()).sort((a,b) => b[1]-a[1]).slice(0, 4).map(([name]) => name);

  state.posts = posts;
  state.topSubs = topSubs;
  state.activeSub = 'All';
  state.query = query || '';

  container.innerHTML = buildRedditCardHTML(state);
  attachRedditCardHandlers(container, state);
}

export function cacheKey(q) { return `redditCardCache:${q || ''}`; }
export function serializeState(state = redditCardState) {
  return { posts: state.posts, topSubs: state.topSubs, activeSub: 'All', query: state.query };
}
export function restoreState(obj, state = redditCardState) {
  if (!obj) return state;
  state.posts = Array.isArray(obj.posts) ? obj.posts : [];
  state.topSubs = Array.isArray(obj.topSubs) ? obj.topSubs : [];
  state.activeSub = obj.activeSub || 'All';
  state.query = obj.query || '';
  return state;
}
