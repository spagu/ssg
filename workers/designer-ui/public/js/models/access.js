// What the signed-in account may do, as pure functions over GET api/me:
// {kind, user: {id, email}, permissions: ["designer-ui:read", …], loginUrl,
// logoutUrl, refreshUrl}. The panel uses them to leave out controls the
// account cannot use (a missing permission means the control is absent, not
// greyed out); the worker checks every permission itself, so this is
// courtesy, never the protection.

export const PERM = {
  read: "designer-ui:read",
  settings: "designer-ui:settings",
  advanced: "designer-ui:advanced",
  theme: "designer-ui:theme",
};

// NOBODY: what the panel assumes when api/me could not be read — nothing.
// designer-ui has one sign-in mode (auth), so there is no "allow all" fallback.
export const NOBODY = Object.freeze({ kind: "user", user: null, permissions: [] });

// capabilities: {read, settings, advanced, theme} as booleans.
export function capabilities(me) {
  const has = (p) => Array.isArray(me?.permissions) && me.permissions.includes(p);
  return { read: has(PERM.read), settings: has(PERM.settings), advanced: has(PERM.advanced), theme: has(PERM.theme) };
}

// editable: may this account change this setting? Plain keys need
// designer-ui:settings; keys that run code, deploy or name a secret also
// designer-ui:advanced (the worker refuses the whole publication otherwise).
export const editable = (entry, caps) => caps.settings && (entry.class === "plain" || caps.advanced);

// needsAdvanced: a value holding a $NAME reference needs designer-ui:advanced
// whatever the key (the worker's rule: a reference reads the build's
// environment, and a key like `variables` would publish it).
export function needsAdvanced(v) {
  if (typeof v === "string") return /\$(?:\{[A-Za-z_][A-Za-z0-9_]*\}|[A-Za-z_][A-Za-z0-9_]*)/.test(v);
  if (Array.isArray(v)) return v.some(needsAdvanced);
  return typeof v === "object" && v !== null && Object.values(v).some(needsAdvanced);
}

// account: who to name in the shell, and where to sign out.
export const account = (me) => (me?.user ? { email: me.user.email || me.user.id, logoutUrl: me.logoutUrl } : null);

// loginTarget: the login page with `next` = where the person is now; null
// when the login URL is missing or leaves this site (`auth` always lives on
// the same host, so anything else is never followed).
export function loginTarget(me, here, origin) {
  if (!me?.loginUrl) return null;
  let url;
  try {
    url = new URL(me.loginUrl, origin);
  } catch {
    return null;
  }
  if (url.origin !== origin) return null;
  url.searchParams.set("next", here);
  return url.pathname + url.search;
}
