// Cloudflare Pages Function: the module-management view (GO-149).
//   GET /api/core/manage[?lang=pl] →
//     {mode, user, lang, writable, readOnly, permission,
//      items: ManageItem[],          every module, hidden ones too, in bar order
//      available: AvailableItem[],   templates the site does not use (registry)
//      history: [{module, status, reason, at}],     newest first
//      changes: [{at, actor, module, action, detail}], newest first
//      retention: {historyDays, changesDays}, limits: {label, note, importBytes}}
//
// readOnly: null when the settings can be changed; "no_database" without
// CORE_DB (everything else of core works as before); "database_error" when
// CORE_DB failed just now. Needs core:manage in the `auth` mode (_manage.ts).

import { intVar, json } from "../_lib";
import type { Env } from "../_lib";
import { readStored, availableItems, manageItems, reading } from "../_manage";
import type { Stored } from "../_manage";
import type { Registry } from "../_registry";
import { route } from "../_route";
import { LIMITS } from "../_settings-check";
import type { Caller } from "../_signin";
import { pickLang } from "../_view";

const EMPTY: Stored = { settings: new Map(), latest: new Map(), history: [], changes: [] };

async function view(request: Request, env: Env, caller: Caller, registry: Registry): Promise<Response> {
  const lang = pickLang(request);
  let stored = EMPTY;
  let readOnly: "no_database" | "database_error" | null = env.CORE_DB ? null : "no_database";
  if (env.CORE_DB) {
    try {
      stored = await readStored(env.CORE_DB);
    } catch {
      readOnly = "database_error";
    }
  }
  return json({
    mode: caller.mode,
    user: caller.user,
    lang,
    writable: readOnly === null,
    readOnly,
    permission: "core:manage",
    items: manageItems(registry, stored.settings, stored.latest, lang),
    available: availableItems(registry),
    history: stored.history,
    changes: stored.changes,
    retention: { historyDays: intVar(env.CORE_HISTORY_DAYS, 90), changesDays: intVar(env.CORE_CHANGES_DAYS, 365) },
    limits: { label: LIMITS.label, note: LIMITS.note, importBytes: LIMITS.importBytes },
  });
}

export const onRequest = route({ GET: reading(view) });
