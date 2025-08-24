import { $, escapeHtml } from './utils.js';

export function renderKnowledge(container, card) {
  if (!container) return;
  if (!card || (!card.title && !card.Title)) { container.innerHTML = ''; return; }
  const title = card.title || card.Title || '';
  const desc = card.description || card.Description || '';
  const extract = card.extract || card.Extract || '';
  // Suppress disambiguation pages (e.g., "FSF may refer to:" or description mentioning disambiguation)
  const isDisambig = /may refer to/i.test(extract) || /disambiguation/i.test(desc) || /disambiguation/i.test(extract);
  if (isDisambig) { container.innerHTML = ''; return; }

  const url = card.url || card.URL || '';
  const thumb = card.thumbnail || card.Thumbnail || '';
  // Build a responsive srcset for Commons logos when possible (…FilePath/…?width=NNN)
  const makeSrcSet = (u) => {
    if (!u) return '';
    try {
      const url = new URL(u);
      if (!url.searchParams.has('width')) return '';
      const widths = [160, 240, 320];
      const items = widths.map(w => {
        const nu = new URL(u);
        nu.searchParams.set('width', String(w));
        return `${nu.toString()} ${w}w`;
      });
      return items.join(', ');
    } catch (_) { return ''; }
  };
  const srcset = makeSrcSet(thumb);
  const facts = card.facts || card.Facts || [];
  const website = card.website || card.Website || '';
  const wikiLogo = 'https://en.wikipedia.org/static/favicon/wikipedia.ico';
  const factsHTML = Array.isArray(facts) && facts.length > 0
    ? (() => {
        const topN = 8;
        const collapsed = facts.slice(0, topN);
        const rest = facts.slice(topN);
        const mkItem = (f) => {
          const k = escapeHtml(String(f.key || f.Key || ''));
          const v = escapeHtml(String(f.value || f.Value || ''));
          if (!k || !v) return '';
          return `<div class="kfact"><dt>${k}</dt><dd>${v}</dd></div>`;
        };
        const collapsedHTML = collapsed.map(mkItem).join('');
        const restHTML = rest.map(mkItem).join('');
        const toggle = rest.length > 0
          ? `<button class="kfacts-toggle" aria-label="Toggle facts" title="Show more" data-expanded="false" type="button" aria-controls="kfacts-extra">▸</button>`
          : '';
        const extraBlock = rest.length
          ? `<dl id="kfacts-extra" class="kfacts-list kfacts-extra" style="display:none">${restHTML}</dl>`
          : '';
        return `<div class="kfacts">
            <div class="kfacts-title-row"><h4 class="kfacts-title">Facts</h4>${toggle}</div>
            <dl class="kfacts-list">
              ${collapsedHTML}
            </dl>
            ${extraBlock}
          </div>`;
      })()
    : '';
  const metaLinks = (website || url)
    ? `<div class="kmeta">
        ${website ? `<a class="kmeta-link" href="${website}" target="_blank" rel="noopener" aria-label="Website">Website</a>` : ''}
        ${url ? `<a class="kmeta-link" href="${url}" target="_blank" rel="noopener" aria-label="Wikipedia">Wikipedia</a>` : ''}
      </div>`
    : '';

  const thumbLink = website || url || '';
  container.innerHTML = `
    <article class="kcard" role="complementary">
      <div class="khead">
        <img class="klogo" src="${wikiLogo}" alt="Wikipedia" />
        <div>
          <h3 class="ktitle">${escapeHtml(title)}</h3>
          ${desc ? `<div class="kdesc">${escapeHtml(desc)}</div>` : ''}
          ${metaLinks}
        </div>
      </div>
      ${thumb ? (
        thumbLink
          ? `<a class="kthumb-link" href="${thumbLink}" target="_blank" rel="noopener"><img class="kthumb" src="${thumb}" ${srcset ? `srcset="${srcset}" sizes="(max-width: 360px) 160px, (max-width: 520px) 240px, 320px"` : ''} alt="${escapeHtml(title)}" loading="eager" fetchpriority="high" decoding="async" /></a>`
          : `<img class="kthumb" src="${thumb}" ${srcset ? `srcset="${srcset}" sizes="(max-width: 360px) 160px, (max-width: 520px) 240px, 320px"` : ''} alt="${escapeHtml(title)}" loading="eager" fetchpriority="high" decoding="async" />`
      ) : ''}
      ${extract ? `<p class="kextract">${escapeHtml(extract)}</p>` : ''}
      ${factsHTML}
    </article>`;

  // Attach toggle for Facts
  const toggleBtn = container.querySelector('.kfacts-toggle');
  const extra = container.querySelector('.kfacts-list.kfacts-extra');
  if (toggleBtn && extra) {
    toggleBtn.addEventListener('click', () => {
      const expanded = toggleBtn.getAttribute('data-expanded') === 'true';
      if (expanded) {
        extra.style.display = 'none';
        toggleBtn.setAttribute('data-expanded', 'false');
        toggleBtn.textContent = '▸';
        toggleBtn.setAttribute('title', 'Show more');
      } else {
        extra.style.display = '';
        toggleBtn.setAttribute('data-expanded', 'true');
        toggleBtn.textContent = '▾';
        toggleBtn.setAttribute('title', 'Show less');
      }
    });
  }
}
