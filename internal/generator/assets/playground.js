// Live examples (GO-116): every block marked data-ssg-playground becomes an
// editor with Run and Reset. The code runs in a sandboxed iframe — scripts
// only, an opaque origin, so it cannot touch this page, its cookies or its
// storage — with the documented package mapped by an import map to the
// module URL the site configured. Both `import { x } from "pkg"` and the
// bare names (the package's exports are also globals) work. console output
// and uncaught errors come back by postMessage. Without JavaScript the block
// stays an ordinary code sample.
(function () {
  'use strict';
  var limitMs = 5000;

  // safeJSON is JSON that can sit inside a <script> element.
  function safeJSON(v) {
    return JSON.stringify(v).replace(/</g, '\\u003c').replace(/\u2028/g, '\\u2028').replace(/\u2029/g, '\\u2029');
  }

  // runnerHTML is the iframe document: the import map, then one module that
  // forwards the console, loads the package, and imports the reader's code.
  function runnerHTML(pkg, mod, code, token) {
    var imports = {};
    imports[pkg] = mod;
    return '<!doctype html><meta charset="utf-8">' +
      '<script type="importmap">' + safeJSON({ imports: imports }) + '<\/script>' +
      '<script type="module">' +
      'const token=' + safeJSON(token) + ',pkg=' + safeJSON(pkg) + ',code=' + safeJSON(code) + ';' +
      'const send=(type,level,text)=>parent.postMessage({token,type,level,text},"*");' +
      'const fmt=v=>{if(typeof v==="string")return v;if(v instanceof Error)return v.name+": "+v.message;' +
      'try{return JSON.stringify(v,null,2)??String(v)}catch(e){return String(v)}};' +
      '["log","info","warn","error","debug"].forEach(l=>{console[l]=(...a)=>send("log",l,a.map(fmt).join(" "))});' +
      'addEventListener("error",e=>send("log","error",fmt(e.error||e.message)));' +
      'addEventListener("unhandledrejection",e=>send("log","error","Uncaught (in promise) "+fmt(e.reason)));' +
      'try{Object.assign(globalThis,await import(pkg))}catch(e){send("log","error","Could not load "+pkg+": "+fmt(e))}' +
      'try{await import(URL.createObjectURL(new Blob([code],{type:"text/javascript"})))}catch(e){send("log","error",fmt(e))}' +
      'setTimeout(()=>send("done"),50);' +
      '<\/script>';
  }

  function button(label) {
    var b = document.createElement('button');
    b.type = 'button';
    b.textContent = label;
    return b;
  }

  function print(out, level, text) {
    var line = document.createElement('span');
    line.className = 'ssg-playground-' + (level === 'error' || level === 'warn' ? level : 'log');
    line.textContent = text + '\n';
    out.appendChild(line);
  }

  function execute(box, code, out, run) {
    var mod = new URL(box.getAttribute('data-module'), document.baseURI).href;
    var token = crypto.getRandomValues(new Uint32Array(2)).join('-');
    var frame = document.createElement('iframe');
    frame.setAttribute('sandbox', 'allow-scripts');
    frame.title = 'Example runner';
    frame.hidden = true;
    frame.srcdoc = runnerHTML(box.getAttribute('data-package'), mod, code, token);
    out.textContent = '';
    out.hidden = false;
    run.disabled = true;
    var timer = setTimeout(function () { finish('Stopped after ' + limitMs / 1000 + ' seconds.'); }, limitMs);
    function onMessage(e) {
      // The runner is a sandboxed frame, so its origin is always the opaque
      // "null"; together with the frame and the per-run token, a message
      // from anywhere else is ignored.
      if (e.origin !== 'null' || e.source !== frame.contentWindow || !e.data || e.data.token !== token) return;
      if (e.data.type === 'done') finish();
      else print(out, e.data.level, String(e.data.text));
    }
    function finish(message) {
      clearTimeout(timer);
      window.removeEventListener('message', onMessage);
      frame.remove();
      run.disabled = false;
      if (message) print(out, 'error', message);
      if (!out.textContent) print(out, 'log', '(no output)');
    }
    window.addEventListener('message', onMessage);
    document.body.appendChild(frame);
  }

  function setup(box) {
    var pre = box.querySelector('pre');
    if (!pre) return;
    var source = pre.textContent.replace(/\n+$/, '');
    var editor = document.createElement('textarea');
    editor.className = 'ssg-playground-editor';
    editor.value = source;
    editor.spellcheck = false;
    editor.rows = Math.min(source.split('\n').length + 1, 24);
    editor.setAttribute('aria-label', 'Example code (editable; Ctrl+Enter runs it)');
    var bar = document.createElement('div');
    bar.className = 'ssg-playground-bar';
    var run = button('Run'), reset = button('Reset');
    run.className = 'ssg-playground-run';
    var out = document.createElement('pre');
    out.className = 'ssg-playground-output';
    out.setAttribute('aria-live', 'polite');
    out.setAttribute('aria-label', 'Output');
    out.hidden = true;
    bar.appendChild(run);
    bar.appendChild(reset);
    pre.hidden = true;
    box.appendChild(editor);
    box.appendChild(bar);
    box.appendChild(out);
    run.addEventListener('click', function () { execute(box, editor.value, out, run); });
    reset.addEventListener('click', function () {
      editor.value = source;
      out.textContent = '';
      out.hidden = true;
    });
    editor.addEventListener('input', function () {
      editor.rows = Math.min(editor.value.split('\n').length + 1, 24);
    });
    editor.addEventListener('keydown', function (e) {
      if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') {
        e.preventDefault();
        run.click();
      }
    });
  }

  if (typeof module === 'object' && module.exports) {
    module.exports = { runnerHTML: runnerHTML };
    return;
  }
  document.querySelectorAll('[data-ssg-playground]').forEach(setup);
})();
