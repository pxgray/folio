(function() {
  const t = localStorage.getItem('theme');
  if (t) {
    document.documentElement.setAttribute('data-theme', t);
    document.getElementById('theme-btn').textContent = t === 'dark' ? '☀' : '☾';
  }
})();
function toggleTheme() {
  const html = document.documentElement;
  const isDark = html.getAttribute('data-theme') === 'dark';
  const next = isDark ? 'light' : 'dark';
  html.setAttribute('data-theme', next);
  localStorage.setItem('theme', next);
  document.getElementById('theme-btn').textContent = isDark ? '☾' : '☀';
}
