(function(){
  // Apply any stored user preference, overriding the system default.
  var stored=localStorage.getItem('theme');
  if(stored) document.documentElement.setAttribute('data-theme',stored);

  var btn=document.getElementById('theme-btn');
  if(!btn) return;

  function currentTheme(){
    return document.documentElement.getAttribute('data-theme')
      ||(matchMedia('(prefers-color-scheme:dark)').matches?'dark':'light');
  }

  // Sync button icon with the effective theme on load.
  btn.textContent=currentTheme()==='dark'?'☀':'☾';

  btn.addEventListener('click',function(){
    btn.blur();
    var isDark=currentTheme()==='dark';
    var next=isDark?'light':'dark';
    document.documentElement.setAttribute('data-theme',next);
    localStorage.setItem('theme',next);
    btn.textContent=isDark?'☾':'☀';
  });
})();
