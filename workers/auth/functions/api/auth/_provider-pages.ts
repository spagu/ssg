// The login-provider parts of the pages (GO-151): the buttons on the login
// and invitation pages, "How you sign in" on the account page, and the notice
// that says why a role's permissions are left out of a session. A leading
// underscore keeps this file out of the Pages route table.
//
// A provider button is a plain form POSTed to /api/auth/oauth/<id>/start —
// it works with JavaScript off. Branding: every provider asks that its NAME
// be shown unaltered with a sign-in verb ("Sign in with Google", "Continue
// with Facebook") and that its logo, when shown, come in its official form.
// This worker loads nothing from the providers' servers and ships none of
// their logos: the buttons are text in the pages' own neutral style, the
// provider's name in the wording its guidelines give (docs/PROVIDERS.md).

import type { Ctx, Who } from "./_context";
import { fill, when } from "./_i18n";
import type { Lang, Strings } from "./_i18n";
import type { MethodsView } from "./_provider-account";
import { providerName } from "./_provider";
import { PASSWORD_METHOD, policyView } from "./_provider-policy";
import { hasFactor } from "./_mfa-factors";
import { factorNotice } from "./_mfa-pages-parts";
import { sessionFactor } from "./_mfa-policy";
import type { SignInMethod } from "./_provider-methods";
import { esc, field, hidden, usernameFor } from "./_pages";
import { accessLinkButton } from "./_access-pages";
import { ACCESS_METHOD } from "./_access-config";
import type { Rules } from "./_pages-errors";

export const providersOf = (methods: SignInMethod[]): SignInMethod[] => methods.filter((m) => m.provider);

// formOrigins: the authorization origins the pages' CSP form-action needs.
// GO-139: plus a method's own origin (Cloudflare Access's team host, where
// the sign-out form of an Access session is sent on).
export const formOrigins = (methods: SignInMethod[]): string[] => [
  ...providersOf(methods).map((m) => (m.provider as NonNullable<SignInMethod["provider"]>).authorizeOrigin(m.setup as NonNullable<SignInMethod["setup"]>)),
  ...methods.flatMap((m) => (m.formOrigin ? [m.formOrigin] : [])),
];

export const methodLabel = (t: Strings, id: string): string => (id === PASSWORD_METHOD ? t.password : providerName(id));

// providerButtons: one form per provider, with the hidden fields given.
export function providerButtons(methods: SignInMethod[], t: Strings, fields: Record<string, string>): string {
  const forms = providersOf(methods).map((m) => {
    const p = m.provider as NonNullable<SignInMethod["provider"]>;
    const text = fill(p.label === "continue" ? t.providerContinue : t.providerSignIn, { provider: p.name });
    const inputs = Object.entries(fields).map(([k, v]) => hidden(k, v)).join("");
    return `<li><form method="post" action="/api/auth/oauth/${esc(p.id)}/start">${inputs}<button class="btn btn-provider" type="submit" data-provider="${esc(p.id)}">${esc(text)}</button></form></li>`;
  });
  return `<ul class="providers">${forms.join("")}</ul>`;
}

// methodsSection: "How you sign in", "Add a way to sign in", "Set a password".
export function methodsSection(v: MethodsView, who: Who, lang: Lang, t: Strings, r: Rules, methods: SignInMethod[], passwordOn: boolean): string {
  const count = (v.password ? 1 : 0) + v.identities.length;
  const cell = (label: string, html: string) => `<td data-label="${esc(label)}">${html}</td>`;
  const row = (id: string, method: string, detail: string, added: string): string => {
    const label = methodLabel(t, method);
    const used = method === v.sessionMethod ? ` <span class="chip chip-current">${esc(t.thisSessionMethod)}</span>` : "";
    const action =
      count > 1
        ? `<form method="post" action="/api/auth/methods/${esc(id)}"><button class="btn" type="submit" aria-label="${esc(fill(t.removeMethodLabel, { method: label }))}">${esc(t.removeMethod)}</button></form>`
        : `<span class="muted">${esc(t.onlyMethod)}</span>`;
    return `<tr class="method">${cell(t.colMethod, esc(label) + used)}${cell(t.colDetail, esc(detail || "—"))}${cell(t.colAdded, esc(added))}<td>${action}</td></tr>`;
  };
  const rows = [
    ...(v.password ? [row(PASSWORD_METHOD, PASSWORD_METHOD, t.ownSecretIsSet, "—")] : []),
    ...v.identities.map((i) => row(i.id, i.provider, i.email, when(lang, i.linkedAt))),
  ];
  const head = [t.colMethod, t.colDetail, t.colAdded, t.colAction].map((h) => `<th scope="col">${esc(h)}</th>`).join("");
  const parts = [
    `<section class="card" aria-labelledby="methods-title"><h2 id="methods-title">${esc(t.methodsHeading)}</h2><p>${esc(t.methodsLead)}</p>
<div class="table-scroll"><table class="data"><thead><tr>${head}</tr></thead><tbody>${rows.join("")}</tbody></table></div></section>`,
  ];
  const available = methods.filter((m) => m.provider && v.available.some((a) => a.id === m.id));
  // GO-139: "Connect Cloudflare Access" is a link to its own confirmation page.
  const access = v.available.some((a) => a.id === ACCESS_METHOD) ? accessLinkButton(lang) : "";
  if (available.length || access) {
    parts.push(`<section class="card" aria-labelledby="add-title"><h2 id="add-title">${esc(t.addMethodHeading)}</h2><p>${esc(fill(t.addMethodLead, { minutes: v.recentMinutes }))}</p>
${available.length ? providerButtons(available, t, { purpose: "link" }) : ""}${access}</section>`);
  }
  if (!v.password && passwordOn) {
    parts.push(`<section class="card" aria-labelledby="setpw-title"><h2 id="setpw-title">${esc(t.setPasswordHeading)}</h2><p>${esc(t.setPasswordLead)}</p>
<form class="stack" method="post" action="/api/auth/methods/set-password">${usernameFor(who.user.email)}
${field({ id: "set-password", name: "password", label: t.newPassword, type: "password", autocomplete: "new-password", hint: fill(t.newPasswordHint, { min: r.min }), minlength: r.min, maxlength: r.max })}
<button class="btn btn-primary" type="submit">${esc(t.setPasswordHeading)}</button>
</form></section>`);
  }
  return parts.join("\n");
}

// policyNotice explains the roles whose permissions the per-role method
// policy (AUTH_ROLE_METHODS) leaves out of this session, and (GO-140) those
// waiting for a second factor, with the link that gets it; "" when none.
export async function policyNotice(ctx: Ctx, who: Who, t: Strings, method: string): Promise<string> {
  const { withheld, needsFactor } = await policyView(ctx.env, who.user.id, method, (await sessionFactor(ctx.env, who.session.id)) !== null);
  if (!withheld.length && !needsFactor.length) return "";
  const lines = withheld.map((w) =>
    w.methods.length
      ? fill(t.policyRole, { role: w.role, methods: w.methods.map((m) => methodLabel(t, m)).join(", "), method: methodLabel(t, method) })
      : t.policyInvalid,
  );
  const items = [...new Set(lines)].map((l) => `<li>${esc(l)}</li>`).join("");
  const factor = factorNotice(t, needsFactor, await hasFactor(ctx.env, who.user.id), new URL(ctx.request.url).pathname);
  return `<div class="callout warning" role="status"><p class="callout-title">${esc(t.policyHeading)}</p><ul class="perms">${items}${factor}</ul></div>`;
}

