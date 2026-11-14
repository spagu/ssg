// What the signed-in account may do (GO-137), as pure functions over the
// answer of GET api/me: {mode, user, permissions: "all" | [...], loginUrl?,
// logoutUrl?, refreshUrl?}. The panel uses them to HIDE what the account
// cannot use; the worker enforces every permission on its own, so a
// hidden button is courtesy, never the protection.
//
// With the password or Cloudflare Access `permissions` is "all": nothing is
// hidden and the panel looks exactly as it always did.

export const READ = "newsletter:read";
export const EDIT = "newsletter:edit";
export const SEND = "newsletter:send";
export const IMPORT = "newsletter:import";
export const EXPORT = "newsletter:export";
export const ERASE = "newsletter:erase";
export const SUPPRESS = "newsletter:suppress";
export const SETTINGS = "newsletter:settings";

// ALL: what a panel that could not ask (api/me failed) assumes. The worker
// still refuses whatever the account may not do.
export const ALL = { mode: "password", user: null, permissions: "all" };

// VIEW_NEEDS: what opening each view needs (null: nothing). The navigation
// entries are the views of the same names; they follow module.json's nav.
export const VIEW_NEEDS = {
  dashboard: READ,
  subscribers: READ,
  subscriber: READ,
  import: IMPORT,
  campaigns: READ,
  campaignNew: EDIT,
  campaign: READ,
  suppressions: READ,
  settings: SETTINGS,
  activity: SETTINGS,
  help: null,
  notFound: null,
};

// ACTION_NEEDS: each button's permission — the API route behind it needs the same.
export const ACTION_NEEDS = {
  save: EDIT,
  duplicate: EDIT,
  delete: EDIT,
  newCampaign: EDIT,
  fromPost: EDIT,
  editSubscriber: EDIT,
  unsubscribe: EDIT,
  test: SEND,
  schedule: SEND,
  unschedule: SEND,
  pause: SEND,
  resume: SEND,
  cancel: SEND,
  uncertain: SEND,
  confirmations: SEND,
  resendConfirmation: SEND,
  exportList: EXPORT,
  exportPerson: EXPORT,
  erase: ERASE,
  suppress: SUPPRESS,
  lift: SUPPRESS,
  importSuppressions: [IMPORT, SUPPRESS],
  smtpTest: SETTINGS,
};

// lacking names the first permission `me` does not hold for `need` (one name,
// a list of them, or null for none), or null when nothing is missing.
export function lacking(me, need) {
  if (!need || me.permissions === "all") return null;
  const list = Array.isArray(need) ? need : [need];
  return list.find((p) => !me.permissions.includes(p)) ?? null;
}

// allows: whether `me` may do the named action (ACTION_NEEDS) or hold a permission.
export const allows = (me, action) => lacking(me, ACTION_NEEDS[action] ?? action) === null;

// viewLacks: the permission missing to open a view, or null.
export const viewLacks = (me, view) => lacking(me, VIEW_NEEDS[view] ?? null);

// navFor keeps the navigation entries the account can open.
export const navFor = (me, names) => names.filter((name) => viewLacks(me, name) === null);

// account: who to name in the shell, and where to sign out — `auth` mode only.
export function account(me) {
  return me.mode === "auth" && me.user ? { email: me.user.email || me.user.id, logoutUrl: me.logoutUrl } : null;
}

// loginTarget: the login page with `next` = where the admin is now (path,
// query and the view after "#"); null outside `auth` mode.
export function loginTarget(me, here) {
  if (me.mode !== "auth" || !me.loginUrl) return null;
  const sep = me.loginUrl.includes("?") ? "&" : "?";
  return `${me.loginUrl}${sep}next=${encodeURIComponent(here)}`;
}

// AUDIT_ACTIONS: every action the audit trail records (src/routes.ts `audit`),
// for the Activity view's filter; each has words under audit.act.<action>.
export const AUDIT_ACTIONS = [
  ...["create", "edit", "duplicate", "from_post", "delete", "test", "schedule", "unschedule", "pause", "resume", "cancel", "uncertain_resend", "uncertain_dismiss"].map((v) => `campaign.${v}`),
  ...["edit", "unsubscribe", "confirmation", "erase"].map((v) => `subscriber.${v}`),
  ...["import", "confirmations"].map((v) => `subscribers.${v}`),
  ...["add", "lift"].map((v) => `suppression.${v}`),
  "suppressions.import",
  "settings.smtp_test",
];

// auditWho: the account an audit entry names, or null when the mode is all
// that is known (the shared password, Cloudflare Access).
export const auditWho = (entry) => (entry.actor ? entry.actor.email || entry.actor.id : null);
