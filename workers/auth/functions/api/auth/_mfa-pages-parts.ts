// Pieces the second factor's pages share (GO-140). A leading underscore keeps
// this file out of the Pages route table.
//
// Accessible Authentication (WCAG 2.2, 3.3.8): the code fields accept paste
// and autofill (autocomplete="one-time-code" — phones offer a code from a
// message or a password manager), take spaces and dashes, and ask for nothing
// to be remembered or worked out — only to copy what the app shows.
// inputmode="numeric" brings up the digit keyboard; there is no pattern
// attribute, which would refuse "123 456".
//
// A passkey needs the browser's WebAuthn API, so its block is `hidden` and
// only auth.js shows it (when the browser has the API). Without JavaScript the
// page offers what works without it: the app code and the backup code.

import { fill } from "./_i18n";
import type { Strings } from "./_i18n";
import { esc, hidden } from "./_pages";

export interface CodeField {
  id: string;
  name: "code" | "backupCode" | "token";
  label: string;
  hint: string;
  error: boolean; // the page's error belongs to this field (described by #notice)
}

// codeField: a labelled one-line field for a code.
export function codeField(f: CodeField): string {
  const numeric = f.name === "code";
  const attrs = [
    `id="${f.id}"`,
    `name="${f.name}"`,
    `type="${f.name === "token" ? "password" : "text"}"`,
    numeric ? 'inputmode="numeric" autocomplete="one-time-code"' : `autocomplete="off" autocapitalize="${f.name === "token" ? "none" : "characters"}"`,
    'spellcheck="false" maxlength="64" required',
    `aria-describedby="${f.id}-hint${f.error ? " notice" : ""}"`,
    f.error ? 'aria-invalid="true"' : "",
  ].filter(Boolean);
  return `<div class="field"><label for="${f.id}">${esc(f.label)}</label><p class="hint" id="${f.id}-hint">${esc(f.hint)}</p><input ${attrs.join(" ")}></div>`;
}

// codeForm: the authenticator-app code, POSTed to `action`.
export const codeForm = (t: Strings, action: string, fields: Record<string, string>, error: boolean): string =>
  `<form class="stack" method="post" action="${action}">${Object.entries(fields).map(([k, v]) => hidden(k, v)).join("")}
${codeField({ id: "mfa-code", name: "code", label: t.mfaCodeLabel, hint: t.mfaCodeHint, error })}
<button class="btn btn-primary" type="submit">${esc(t.mfaVerify)}</button></form>`;

// backupBlock: "Use a backup code", folded unless it is the one in use.
export const backupBlock = (t: Strings, action: string, fields: Record<string, string>, open: boolean, error: boolean): string =>
  `<details class="card fold"${open ? " open" : ""}><summary>${esc(t.mfaBackupHeading)}</summary>
<form class="stack" method="post" action="${action}">${Object.entries(fields).map(([k, v]) => hidden(k, v)).join("")}
${codeField({ id: "mfa-backup", name: "backupCode", label: t.mfaBackupLabel, hint: t.mfaBackupHint, error })}
<button class="btn" type="submit">${esc(t.mfaVerify)}</button></form></details>`;

// passkeyBlock: a plain form whose hidden `passkey` field auth.js fills with
// the answer of navigator.credentials.get() — called with `options`, issued
// for this page (a single-use challenge) — before it is sent. The texts it
// shows come from data attributes, in the page's language.
export const passkeyBlock = (t: Strings, action: string, fields: Record<string, string>, options: unknown): string =>
  `<section class="card" aria-labelledby="passkey-title" data-passkey="get" data-options="${esc(JSON.stringify(options))}" data-working="${esc(t.mfaPasskeyWorking)}" data-failed="${esc(t.mfaPasskeyFailed)}" hidden>
<h2 id="passkey-title">${esc(t.mfaPasskeyHeading)}</h2><form method="post" action="${action}">${Object.entries(fields).map(([k, v]) => hidden(k, v)).join("")}${hidden("passkey", "")}
<button class="btn btn-primary" type="submit">${esc(t.mfaPasskeyButton)}</button></form>
<p class="hint" data-passkey-status role="status" aria-live="polite"></p></section>`;

// kindLabel: a factor kind in words.
export const kindLabel = (t: Strings, kind: string): string => (kind === "passkey" ? t.mfaKindPasskey : t.mfaKindTotp);

// rolesText: role names for a sentence.
export const rolesText = (roles: string[]): string => roles.join(", ");

// factorNotice: the policy notice's second-factor line (_provider-pages.ts
// policyNotice): which roles wait for a second factor, and the link that
// gets it — enrolment for an account without one, a step-up otherwise.
export function factorNotice(t: Strings, roles: string[], hasFactor: boolean, back: string): string {
  if (!roles.length) return "";
  const href = hasFactor ? `/auth/mfa/step-up?next=${encodeURIComponent(back)}` : `/auth/mfa/enrol?next=${encodeURIComponent(back)}`;
  return `<li>${esc(fill(t.mfaPolicyRoles, { roles: rolesText(roles) }))} <a href="${esc(href)}">${esc(hasFactor ? t.mfaPolicyStepUp : t.mfaPolicyEnrol)}</a></li>`;
}
