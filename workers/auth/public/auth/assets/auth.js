// The sign-in pages work without this file: every action is a plain form.
// With JavaScript it adds three conveniences, nothing more:
//   1. an error message from the server takes focus, so it is read out first;
//   2. each password field gets a "Show password" button (WCAG 3.3.8: checking
//      what you typed beats retyping it);
//   3. a form sent once is not sent again by a second click.
// The button texts come from the page (data-show-password / data-hide-password
// on <body>), in the page's language. No network, no storage, no tracking.
"use strict";

(function () {
  const body = document.body;

  const notice = document.querySelector("#notice[role=alert]");
  if (notice) notice.focus();

  let n = 0;
  for (const input of document.querySelectorAll('input[type="password"]')) {
    const wrap = document.createElement("div");
    wrap.className = "pw";
    input.replaceWith(wrap);
    wrap.append(input);
    const button = document.createElement("button");
    button.type = "button";
    button.className = "btn";
    button.id = "pw-toggle-" + ++n;
    button.setAttribute("aria-controls", input.id);
    button.setAttribute("aria-pressed", "false");
    button.textContent = body.dataset.showPassword || "Show password";
    button.addEventListener("click", () => {
      const show = input.type === "password";
      input.type = show ? "text" : "password";
      button.setAttribute("aria-pressed", String(show));
      button.textContent = show ? body.dataset.hidePassword || "Hide password" : body.dataset.showPassword || "Show password";
    });
    wrap.append(button);
  }

  for (const form of document.querySelectorAll("form")) {
    form.addEventListener("submit", (event) => {
      if (form.dataset.sent) {
        event.preventDefault();
        return;
      }
      form.dataset.sent = "1";
      // A shown password goes back to dots before the page leaves.
      for (const input of form.querySelectorAll(".pw input")) if (input.type === "text") input.type = "password";
      const submit = event.submitter || form.querySelector('button[type="submit"]');
      if (submit) submit.setAttribute("aria-busy", "true");
    });
  }
})();

// GO-140: passkeys on the second-factor pages. A [data-passkey] block is a
// plain form, `hidden` in the HTML because it needs the browser's WebAuthn
// API; with the API it is shown, and sending it first runs the ceremony with
// the options the page carries (data-options: a single-use challenge issued
// for this page; binary fields base64url) and puts the browser's answer in the
// form's hidden field — `passkey` (sign-in, step-up) or `credential`
// (enrolment) — as JSON. Nothing else leaves the page; no request of its own.
(function () {
  if (!window.PublicKeyCredential || !navigator.credentials) return;
  const bytes = (s) => Uint8Array.from(atob(s.replace(/-/g, "+").replace(/_/g, "/") + "===".slice((s.length + 3) % 4)), (c) => c.charCodeAt(0));
  const b64 = (buf) => btoa(String.fromCharCode(...new Uint8Array(buf))).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");

  async function answer(kind, options) {
    const publicKey = JSON.parse(options);
    publicKey.challenge = bytes(publicKey.challenge);
    for (const c of publicKey.allowCredentials || publicKey.excludeCredentials || []) c.id = bytes(c.id);
    if (kind === "get") {
      const cred = await navigator.credentials.get({ publicKey });
      const r = cred.response;
      return { id: cred.id, type: cred.type, response: { clientDataJSON: b64(r.clientDataJSON), authenticatorData: b64(r.authenticatorData), signature: b64(r.signature) } };
    }
    publicKey.user.id = bytes(publicKey.user.id);
    const cred = await navigator.credentials.create({ publicKey });
    const r = cred.response;
    const transports = typeof r.getTransports === "function" ? r.getTransports() : [];
    return { id: cred.id, type: cred.type, response: { clientDataJSON: b64(r.clientDataJSON), attestationObject: b64(r.attestationObject), transports } };
  }

  for (const block of document.querySelectorAll("[data-passkey]")) {
    const form = block.querySelector("form");
    const field = form.querySelector(block.dataset.passkey === "get" ? 'input[name="passkey"]' : 'input[name="credential"]');
    const status = block.querySelector("[data-passkey-status]");
    block.hidden = false;
    form.addEventListener("submit", (event) => {
      if (field.value) return; // the answer is in: let the form go
      // The handler above marked the form sent; it is not, yet.
      event.preventDefault();
      delete form.dataset.sent;
      status.textContent = block.dataset.working;
      answer(block.dataset.passkey, block.dataset.options)
        .then((value) => {
          field.value = JSON.stringify(value);
          form.requestSubmit();
        })
        .catch(() => {
          for (const b of form.querySelectorAll("[aria-busy]")) b.removeAttribute("aria-busy");
          status.textContent = block.dataset.failed;
        });
    });
  }
})();
