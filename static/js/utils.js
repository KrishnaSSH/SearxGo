// Shared utilities for SearxGO frontend

export const $ = (sel, root = document) => root.querySelector(sel);
export const $$ = (sel, root = document) => Array.from(root.querySelectorAll(sel));

export function escapeHtml(s) {
  return String(s).replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;','\'':'&#39;'}[c]));
}

export function capitalize(s) {
  if (!s) return s;
  return s.charAt(0).toUpperCase() + s.slice(1);
}

export function formatMs(ms) {
  if (ms == null) return '';
  if (ms < 1000) return `${ms} ms`;
  return `${(ms/1000).toFixed(1)} s`;
}

export function prettyEngineName(name) {
  if (!name) return '';
  const map = { duckduckgo: 'DuckDuckGo', reddit: 'Reddit', bing: 'Bing', google: 'Google' };
  return map[String(name).toLowerCase()] || name;
}

// Derive readable site name from host (handles subdomains and some ccTLD 2-part suffixes)
export function deriveSiteName(host) {
  if (!host) return '';
  const h = host.toLowerCase().replace(/^www\./, '');
  const parts = h.split('.');
  if (parts.length === 1) return capitalize(parts[0]);
  const twoPartSuffixes = new Set([
    'co.uk','org.uk','gov.uk','ac.uk',
    'com.au','net.au','org.au',
    'co.in','ac.in','gov.in','res.in',
    'com.br','com.mx','com.tr','com.cn','com.hk'
  ]);
  const last2 = parts.slice(-2).join('.');
  let coreIdx = parts.length - 2; // default second-level
  if (twoPartSuffixes.has(last2) && parts.length >= 3) {
    coreIdx = parts.length - 3; // e.g., bbc.co.uk -> bbc
  }
  let core = parts[coreIdx] || parts[0];
  const brandMap = {
    'wikipedia': 'Wikipedia',
    'github': 'GitHub',
    'stackexchange': 'Stack Exchange',
    'stackoverflow': 'Stack Overflow',
    'reddit': 'Reddit',
    'bbc': 'BBC',
    'medium': 'Medium',
    'nytimes': 'NYTimes',
    'cloudflare': 'Cloudflare',
    'google': 'Google',
    'bing': 'Bing',
    'duckduckgo': 'DuckDuckGo'
  };
  if (brandMap[core]) return brandMap[core];
  return capitalize(core);
}
