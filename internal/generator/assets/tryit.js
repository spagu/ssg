// "Try it" for REST operations (GO-118). Each element marked data-ssg-tryit
// carries its operation as JSON — method, path, servers, parameters, body
// and the security schemes it accepts — and becomes a form that sends the
// request with fetch and shows the status, time, headers, body and the same
// request as a curl command.
//
// Credentials are typed once per page and shared by every operation on it.
// They live in this script's memory only: never in storage, never in the
// curl command (which shows a placeholder), gone when the page closes.
(function () {
  'use strict';

  // fill puts path parameters into the template, encoded.
  function fill(path, values) {
    return path.replace(/\{([^}]+)\}/g, function (m, name) {
      return values[name] === undefined || values[name] === '' ? m : encodeURIComponent(values[name]);
    });
  }

  // base64 is btoa for any Unicode text.
  function base64(text) {
    var bytes = new TextEncoder().encode(text), bin = '';
    for (var i = 0; i < bytes.length; i++) bin += String.fromCharCode(bytes[i]);
    return btoa(bin);
  }

  // buildRequest turns the operation and the form's values into what fetch
  // needs. masked replaces secrets with placeholders, for display. It returns
  // {url, method, headers, body, missing, notes}.
  function buildRequest(op, server, values, creds, masked) {
    var missing = [], notes = [], headers = {}, query = [];
    (op.params || []).forEach(function (p) {
      var v = values[p.in + ':' + p.name];
      if (v === undefined || v === '') {
        if (p.required) missing.push(p.name);
        return;
      }
      if (p.in === 'query') query.push([p.name, v]);
      else if (p.in === 'header') headers[p.name] = v;
      else if (p.in === 'cookie') notes.push('Cookie parameter ' + p.name + ' cannot be set from the browser.');
    });
    var pathValues = {};
    (op.params || []).forEach(function (p) {
      if (p.in === 'path') pathValues[p.name] = values['path:' + p.name];
    });
    (op.security || []).forEach(function (s) {
      var c = creds[s.name] || {};
      if (s.type === 'apiKey') {
        if (!c.value) return;
        var key = masked ? '<' + s.paramName + '>' : c.value;
        if (s.in === 'header') headers[s.paramName] = key;
        else if (s.in === 'query') query.push([s.paramName, key]);
        else notes.push('API key ' + s.paramName + ' is sent as a cookie, which the browser does not let a page set.');
      } else if (s.type === 'http' && /^basic$/i.test(s.scheme)) {
        if (c.user || c.password) headers.Authorization = 'Basic ' + (masked ? '<credentials>' : base64((c.user || '') + ':' + (c.password || '')));
      } else if (c.value) {
        headers.Authorization = 'Bearer ' + (masked ? '<token>' : c.value);
      }
    });
    var body = values.body;
    if (op.body && body !== undefined && body !== '') {
      headers['Content-Type'] = op.body.type;
    } else {
      if (op.body && op.body.required) missing.push('request body');
      body = undefined;
    }
    var url = server.replace(/\/+$/, '') + fill(op.path, pathValues);
    if (query.length) {
      url += (url.indexOf('?') >= 0 ? '&' : '?') + query.map(function (q) {
        return encodeURIComponent(q[0]) + '=' + (masked && /^<.*>$/.test(q[1]) ? q[1] : encodeURIComponent(q[1]));
      }).join('&');
    }
    if (/\{[^}]+\}/.test(url)) missing = missing.concat((url.match(/\{[^}]+\}/g) || []).map(function (m) { return m.slice(1, -1); }));
    return { url: url, method: op.method, headers: headers, body: body, missing: unique(missing), notes: notes };
  }

  function unique(list) {
    return list.filter(function (v, i) { return list.indexOf(v) === i; });
  }

  // quote is a shell single-quoted string.
  function quote(s) {
    return "'" + String(s).replace(/'/g, "'\\''") + "'";
  }

  // curlOf is the request as a command line.
  function curlOf(req) {
    var parts = ['curl', '-X', req.method, quote(req.url)];
    Object.keys(req.headers).forEach(function (k) { parts.push('-H', quote(k + ': ' + req.headers[k])); });
    if (req.body !== undefined) parts.push('--data-raw', quote(req.body));
    return parts.join(' ');
  }

  // pretty indents a JSON body; anything else is returned as it is.
  function pretty(text, type) {
    if (!/json/i.test(type || '')) return text;
    try { return JSON.stringify(JSON.parse(text), null, 2); } catch (e) { return text; }
  }

  if (typeof module === 'object' && module.exports) {
    module.exports = { buildRequest: buildRequest, curlOf: curlOf, pretty: pretty, base64: base64 };
    return;
  }

  var creds = {}, inputs = {};

  function el(tag, attrs, text) {
    var e = document.createElement(tag);
    Object.keys(attrs || {}).forEach(function (k) { e.setAttribute(k, attrs[k]); });
    if (text !== undefined) e.textContent = text;
    return e;
  }

  var seq = 0;
  function field(labelText, hint, control) {
    var id = 'ssg-tryit-' + (++seq), label = el('label', { for: id });
    control.id = id;
    label.appendChild(el('span', {}, labelText));
    if (hint) label.appendChild(el('small', {}, hint));
    label.appendChild(control);
    return label;
  }

  // credentialFields are the inputs of one security scheme. Each input is
  // shared: typing in one operation's console fills the others on the page.
  function credentialFields(s) {
    var out = [];
    function bind(key, labelText, type) {
      var input = el('input', { type: type, autocomplete: 'off', spellcheck: 'false' });
      creds[s.name] = creds[s.name] || {};
      input.value = creds[s.name][key] || '';
      (inputs[s.name + '.' + key] = inputs[s.name + '.' + key] || []).push(input);
      input.addEventListener('input', function () {
        creds[s.name][key] = input.value;
        inputs[s.name + '.' + key].forEach(function (o) { if (o !== input) o.value = input.value; });
      });
      out.push(field(labelText, s.description || '', input));
    }
    if (s.type === 'http' && /^basic$/i.test(s.scheme)) {
      bind('user', s.name + ' — user name', 'text');
      bind('password', s.name + ' — password', 'password');
    } else if (s.type === 'apiKey') {
      bind('value', s.name + ' (' + s.paramName + ' in ' + s.in + ')', 'password');
    } else {
      bind('value', s.name + ' — bearer token', 'password');
    }
    return out;
  }

  function setup(box) {
    var op;
    try { op = JSON.parse(box.getAttribute('data-ssg-tryit')); } catch (e) { return; }
    var panelId = 'ssg-tryit-panel-' + (++seq);
    var toggle = el('button', { type: 'button', 'aria-expanded': 'false', 'aria-controls': panelId }, 'Try it');
    var panel = el('div', { id: panelId, class: 'ssg-tryit-panel' });
    panel.hidden = true;
    toggle.addEventListener('click', function () {
      panel.hidden = !panel.hidden;
      toggle.setAttribute('aria-expanded', panel.hidden ? 'false' : 'true');
    });

    var servers = (op.servers && op.servers.length) ? op.servers : [location.origin];
    var serverSelect = el('select');
    servers.forEach(function (s) { serverSelect.appendChild(el('option', { value: s }, s)); });
    panel.appendChild(field('Server', '', serverSelect));

    var controls = {};
    if (op.params && op.params.length) {
      var fs = el('fieldset');
      fs.appendChild(el('legend', {}, 'Parameters'));
      op.params.forEach(function (p) {
        var c;
        if (p.enum && p.enum.length) {
          c = el('select');
          if (!p.required) c.appendChild(el('option', { value: '' }, '—'));
          p.enum.forEach(function (v) { c.appendChild(el('option', { value: String(v) }, String(v))); });
        } else {
          c = el('input', { type: 'text' });
          if (p.example !== undefined && p.example !== null) c.value = String(p.example);
        }
        if (p.required) c.setAttribute('aria-required', 'true');
        controls[p.in + ':' + p.name] = c;
        fs.appendChild(field(p.name + ' (' + p.in + (p.required ? ', required' : '') + ')', p.description || '', c));
      });
      panel.appendChild(fs);
    }
    var bodyArea;
    if (op.body) {
      bodyArea = el('textarea', { rows: '8', spellcheck: 'false' });
      bodyArea.value = op.body.example || '';
      panel.appendChild(field('Request body (' + op.body.type + ')', op.body.required ? 'required' : '', bodyArea));
    }
    if (op.security && op.security.length) {
      var auth = el('fieldset');
      auth.appendChild(el('legend', {}, 'Authorization'));
      op.security.forEach(function (s) { credentialFields(s).forEach(function (f) { auth.appendChild(f); }); });
      auth.appendChild(el('p', { class: 'ssg-tryit-note' }, 'Kept in this page only, not saved anywhere.'));
      panel.appendChild(auth);
    }

    var send = el('button', { type: 'button', class: 'ssg-tryit-send' }, 'Send request');
    var bar = el('div', { class: 'ssg-tryit-bar' });
    bar.appendChild(send);
    panel.appendChild(bar);
    var result = el('div', { class: 'ssg-tryit-response', 'aria-live': 'polite' });
    panel.appendChild(result);

    function values() {
      var v = {};
      Object.keys(controls).forEach(function (k) { v[k] = controls[k].value; });
      if (bodyArea) v.body = bodyArea.value;
      return v;
    }

    function show(statusText, parts) {
      result.textContent = '';
      result.appendChild(el('p', { class: 'ssg-tryit-status' }, statusText));
      parts.forEach(function (p) {
        if (p.length > 1 && !p[1]) return;
        result.appendChild(el('p', { class: 'ssg-tryit-note' }, p[0]));
        if (p.length > 1) result.appendChild(el('pre', {}, p[1]));
      });
    }

    send.addEventListener('click', function () {
      var v = values(), req = buildRequest(op, serverSelect.value, v, creds, false);
      var shown = curlOf(buildRequest(op, serverSelect.value, v, creds, true));
      Object.keys(controls).forEach(function (k) { controls[k].removeAttribute('aria-invalid'); });
      if (req.missing.length) {
        req.missing.forEach(function (name) {
          Object.keys(controls).forEach(function (k) { if (k.split(':')[1] === name) controls[k].setAttribute('aria-invalid', 'true'); });
        });
        show('Fill in: ' + req.missing.join(', '), [['curl', shown]]);
        return;
      }
      send.disabled = true;
      var started = performance.now();
      fetch(req.url, { method: req.method, headers: req.headers, body: req.body, mode: 'cors', credentials: 'omit' })
        .then(function (res) {
          return res.text().then(function (text) {
            var ms = Math.round(performance.now() - started), head = [];
            res.headers.forEach(function (val, key) { head.push(key + ': ' + val); });
            show(res.status + ' ' + res.statusText + ' · ' + ms + ' ms', [
              ['Response body', pretty(text.slice(0, 200000), res.headers.get('content-type'))],
              ['Response headers', head.join('\n')],
              ['curl', shown]].concat(req.notes.map(function (n) { return [n]; })));
          });
        })
        .catch(function (err) {
          show('No response', [
            ['The request did not reach the API, or the browser blocked the answer. If the API works from curl, ' +
              'its server has to allow this site in CORS (Access-Control-Allow-Origin). ' + (err && err.message ? '(' + err.message + ')' : '')],
            ['curl', shown]]);
        })
        .then(function () { send.disabled = false; }, function () { send.disabled = false; });
    });

    box.appendChild(toggle);
    box.appendChild(panel);
  }

  document.querySelectorAll('[data-ssg-tryit]').forEach(setup);
})();
