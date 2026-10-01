package generator

// listenScript drives every Listen button on a page (1.8.65, compact in 1.8.66).
//
// A button with data-audio-src plays that MP3 — the build-time recording —
// and a second press pauses it. A button without one reads the article with
// speechSynthesis, paragraph by paragraph, because Chrome stops a single long
// utterance after about 15 seconds. Either way the label switches to "Stop",
// aria-pressed reports the state, and the button stays hidden where it could
// not work (no JavaScript; for speech, no Web Speech API).
//
// For speech it picks the voice instead of taking the browser's default,
// usually the oldest and most robotic one installed. Among the voices for the
// page language it prefers, in order: a name containing data-voice, neural
// voices (Edge "Natural"/"Online", Chrome "Google", Apple "Premium"/"Enhanced"/
// "Siri"), then an exact language-region match.
const listenScript = `<script data-ssg-listen-script>(function(){` +
	`var s=window.speechSynthesis,canSpeak=!!(s&&window.SpeechSynthesisUtterance);if(canSpeak)s.getVoices();` +
	`function pick(lang,want){var p=(lang||'').toLowerCase(),base=p.split('-')[0],best=null,top=-1;` +
	`s.getVoices().forEach(function(x){var l=(x.lang||'').toLowerCase().replace('_','-');` +
	`if(base&&l.split('-')[0]!==base)return;var n=x.name,sc=0;` +
	`if(want&&n.toLowerCase().indexOf(want.toLowerCase())>=0)sc+=100;` +
	`if(/natural|neural|online/i.test(n))sc+=50;if(/google/i.test(n))sc+=40;` +
	`if(/premium|enhanced|siri/i.test(n))sc+=30;if(l===p)sc+=10;` +
	`if(sc>top){top=sc;best=x;}});return best;}` +
	`document.querySelectorAll('button[data-ssg-listen]').forEach(function(b){` +
	`var src=b.getAttribute('data-audio-src'),text=b.querySelector('.ssg-listen-label')||b,label=text.textContent,` +
	`stopText=b.getAttribute('data-stop')||'Stop',audio=null;if(!src&&!canSpeak)return;b.hidden=false;` +
	`function set(on){b.setAttribute('aria-pressed',on?'true':'false');text.textContent=on?stopText:label;}` +
	`b.addEventListener('click',function(){` +
	`if(src){if(!audio){audio=new Audio(src);audio.onended=function(){set(false);};}` +
	`if(audio.paused){audio.play();set(true);}else{audio.pause();set(false);}return;}` +
	`if(s.speaking){s.cancel();set(false);return;}` +
	`var root=b.closest('article')||document.querySelector('main')||document.body;` +
	`var parts=[].map.call(root.querySelectorAll('h1,h2,h3,h4,p,li,blockquote,td'),function(e){return e.contains(b)?'':e.innerText.trim();}).filter(Boolean);` +
	`var lang=document.documentElement.lang||'',v=pick(lang,b.getAttribute('data-voice')||'');` +
	`parts.forEach(function(t,i){var u=new SpeechSynthesisUtterance(t);if(lang)u.lang=lang;if(v)u.voice=v;` +
	`if(i===parts.length-1){u.onend=function(){set(false);};u.onerror=u.onend;}s.speak(u);});set(true);});});` +
	`window.addEventListener('pagehide',function(){if(canSpeak)s.cancel();});})();</script>`
