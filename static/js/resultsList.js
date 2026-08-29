import { escapeHtml, deriveSiteName, prettyEngineName, safeUrl, attrUrl } from './utils.js';

export function renderResults(container, items) {
  if (!container) return;
  container.innerHTML = '';
  if (!Array.isArray(items) || items.length === 0) {
    container.innerHTML = '<p>No results.</p>';
    return;
  }
  const frag = document.createDocumentFragment();
  for (const r of items) {
    const li = document.createElement('li');
    li.className = 'results-item';
    const fav = safeUrl(r.Favicon || r.favicon || '');
    const title = r.Title || r.title || r.URL || r.url || '';
    const urlStr = safeUrl(r.URL || r.url || '');
    const snippet = r.Snippet || r.snippet || '';
    const engineRaw = r.Engine || r.engine || '';
    const enginePretty = prettyEngineName(engineRaw);

    let host = '', path = '', displayUrl = urlStr;
    try {
      const u = new URL(urlStr);
      host = u.host;
      path = (u.pathname + u.search + u.hash) || '/';
      displayUrl = (u.protocol ? u.protocol + '//' : '') + host;
    } catch (_) {}

    const siteName = deriveSiteName(host);
    if (fav) li.classList.add('has-fav');
    const hrefAttr = escapeHtml(urlStr) || '#';
    li.innerHTML = `
      <div class="site-line">
        ${fav ? `<img class="fav" src="${escapeHtml(fav)}" alt="">` : ''}
        <div class="site-meta">
          <span class="site-name">${escapeHtml(siteName || host || '')}</span>
          <div class="url u-ellipsis u-text-secondary">${escapeHtml(displayUrl)}</div>
        </div>
      </div>
      <div class="title-row">
        <h3 class="title"><a href="${hrefAttr}" target="_blank" rel="noopener noreferrer">${escapeHtml(title)}</a></h3>
        ${enginePretty ? `<div class="meta"><span class="engine-badge u-badge" data-engine="${escapeHtml(engineRaw.toLowerCase())}">${escapeHtml(enginePretty)}</span></div>` : ''}
      </div>
      ${snippet ? `<p class="snippet">${escapeHtml(snippet)}</p>` : ''}
    `;
    frag.appendChild(li);
  }
  container.appendChild(frag);
}
