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

(function(){
  var navBtn=document.getElementById('nav-btn');
  var navOverlay=document.getElementById('nav-overlay');
  var sidebar=document.getElementById('sidebar');
  var navClose=document.getElementById('nav-close');
  if(!navBtn||!sidebar) return;

  function openNav(){
    sidebar.classList.add('is-open');
    if(navOverlay) navOverlay.classList.add('is-active');
    document.body.style.overflow='hidden';
  }
  function closeNav(){
    sidebar.classList.remove('is-open');
    if(navOverlay) navOverlay.classList.remove('is-active');
    document.body.style.overflow='';
    navBtn.focus();
  }

  navBtn.addEventListener('click',openNav);
  if(navOverlay) navOverlay.addEventListener('click',closeNav);
  if(navClose) navClose.addEventListener('click',closeNav);
  sidebar.querySelectorAll('a').forEach(function(a){
    a.addEventListener('click',closeNav);
  });
})();
