document.addEventListener('DOMContentLoaded', () => {
  const form = document.getElementById('search_form');
  const input = document.getElementById('q');
  const resultsEl = document.getElementById('results');
  if (!form || !input || !resultsEl) return;

  function escapeHtml(s) {
    return String(s).replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;','\'':'&#39;'}[c]));
  }

  function render(items) {
    resultsEl.innerHTML = '';
    if (!items || !items.length) {
      resultsEl.innerHTML = '<p>No results.</p>';
      return;
    }
    const ul = document.createElement('ul');
    ul.style.listStyle = 'none';
    ul.style.padding = '0';
    items.forEach(r => {
      const li = document.createElement('li');
      li.className = 'result';
      li.innerHTML = `
        <div class="result__title">
          <a href="${r.URL}" target="_blank" rel="noopener noreferrer">${escapeHtml(r.Title || r.URL || '')}</a>
        </div>
        ${r.Snippet ? `<div class="result__snippet">${escapeHtml(r.Snippet)}</div>` : ''}
        <div class="result__meta">${r.Engine ? `<span>${escapeHtml(r.Engine)}</span>` : ''}</div>
      `;
      ul.appendChild(li);
    });
    resultsEl.appendChild(ul);
  }

  form.addEventListener('submit', async (e) => {
    e.preventDefault();
    const q = (input.value || '').trim();
    if (!q) return;
    resultsEl.innerHTML = '<p>Searching…</p>';
    try {
      const u = new URL(window.location.origin + '/search');
      u.searchParams.set('q', q);
      u.searchParams.set('page', '1');
      u.searchParams.set('size', '10');
      const res = await fetch(u.toString());
      if (!res.ok) throw new Error('HTTP ' + res.status);
      const data = await res.json();
      render(data);
    } catch (err) {
      console.error(err);
      resultsEl.innerHTML = '<p>Search failed.</p>';
    }
  });
});
