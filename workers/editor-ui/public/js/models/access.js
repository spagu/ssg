// What the signed-in account may do here, as pure functions over GET api/me:
// {kind, user: {id, email}, permissions: ["editor-ui:read", …], loginUrl,
// logoutUrl, refreshUrl}. The panel leaves out a control the account cannot
// use (a missing permission means the control is absent, not greyed out);
// the worker checks every permission itself, so this is courtesy, never the
// protection.

export const PERM = {
  read: "editor-ui:read",
  publish: "editor-ui:publish",
  media: "editor-ui:media",
  delete: "editor-ui:delete",
};

// NOBODY: what the panel assumes when api/me could not be read — nothing.
export const NOBODY = Object.freeze({ kind: "user", user: null, permissions: [] });

// capabilities: {read, publish, media, delete} as booleans. Adding images
// is part of a publication, so it needs publish as well as media.
export function capabilities(me) {
  const has = (p) => Array.isArray(me?.permissions) && me.permissions.includes(p);
  return { read: has(PERM.read), publish: has(PERM.publish), media: has(PERM.publish) && has(PERM.media), delete: has(PERM.publish) && has(PERM.delete) };
}

// account: who to name in the shell, and where to sign out.
export const account = (me) => (me?.user ? { email: me.user.email || me.user.id, logoutUrl: me.logoutUrl } : null);

// loginTarget: the login page with `next` = where the person is now; null
// when the login URL is missing or leaves this site (`auth` always lives on
// the same host, so anything else is never followed — no open redirect).
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
