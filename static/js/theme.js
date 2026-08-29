// Light/dark theme handling. The no-flash inline script in each page's <head>
// applies a saved theme before first paint; this module keeps the toggle
// buttons in sync and persists changes.

const KEY = 'searxgo.theme';

function stored() {
  try {
    const v = localStorage.getItem(KEY);
    return v === 'dark' || v === 'light' ? v : null;
  } catch (_) { return null; }
}

function systemTheme() {
  return window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
}

function currentTheme() {
  return document.documentElement.getAttribute('data-theme') || stored() || systemTheme();
}

function syncButtons(theme) {
  const dark = theme === 'dark';
  document.querySelectorAll('[data-theme-toggle]').forEach((btn) => {
    btn.setAttribute('aria-pressed', String(dark));
    btn.setAttribute('title', dark ? 'Switch to light mode' : 'Switch to dark mode');
    const ico = btn.querySelector('.ico');
    if (ico) ico.textContent = dark ? '☀' : '☾';
  });
}

function setTheme(theme) {
  document.documentElement.setAttribute('data-theme', theme);
  try { localStorage.setItem(KEY, theme); } catch (_) {}
  syncButtons(theme);
}

export function initTheme() {
  syncButtons(currentTheme());
  document.addEventListener('click', (e) => {
    const btn = e.target.closest('[data-theme-toggle]');
    if (!btn) return;
    e.preventDefault();
    setTheme(currentTheme() === 'dark' ? 'light' : 'dark');
  });
  // React to OS changes only while the user hasn't made an explicit choice.
  if (window.matchMedia) {
    window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', () => {
      if (!stored()) syncButtons(systemTheme());
    });
  }
}
