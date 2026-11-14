// Cloudflare Pages Function: the modules this caller may see.
//   GET /api/core/modules[?lang=pl] → {mode, user, lang, home, signOut,
//                                      brand, items: ModuleView[]}
//
// brand: {name, logo, logoDark} from admin_theme: in .ssg.yaml (GO-152), or
// null — the bar then shows its own default name.
// home: the admin home page (core's panel). signOut: in the `auth` mode, the
// `auth` worker's sign-out route on this host, else null (_visible.ts). The
// shared navigation (public/core/nav.js) is drawn from this answer.
//
// Each item: id, kind, title and description in the asked language (English
// when the module has no such text), version, panel and navigation entries as
// absolute paths on this host, and a status hint:
//   registered        the manifest came with the build (the registry)
//   live              the module was asked for its manifest just now (a
//                     standalone Worker the build could not read)
//   unreachable       it was asked and did not answer usefully
//   invalid_manifest  its manifest is refused by the reader
// A module that refuses the caller when asked is left out: the caller would
// only find a locked door behind its link. Which modules are left out is a
// courtesy, not protection — see _view.ts and _visible.ts.
//
// With CORE_DB, the module settings (GO-149, _settings.ts) apply for everyone:
// hidden modules are left out, the rest come in the set order, and a label
// for the asked language replaces the title. Without it (or when it fails),
// the items are as the build listed them.

import { json } from "./_lib";
import type { Env } from "./_lib";
import { loadRegistry } from "./_registry";
import { route } from "./_route";
import { arrange, readState, relabel } from "./_settings";
import { brandOf } from "./_theme";
import type { Caller } from "./_signin";
import { pickLang } from "./_view";
import { HOME, signOutPath, visibleModules } from "./_visible";

async function list(request: Request, env: Env, caller: Caller): Promise<Response> {
  const loaded = await loadRegistry();
  if (!loaded.ok) return loaded.response;
  const lang = pickLang(request);
  const [visible, state] = await Promise.all([visibleModules(request, env, caller, lang, loaded.registry), readState(env)]);
  const items = arrange(visible, (v) => v.entry.id, state?.settings).map((v) => relabel(v.view, state?.settings, lang));
  const signOut = signOutPath(caller, loaded.registry, env, new URL(request.url).origin);
  return json({ mode: caller.mode, user: caller.user, lang, home: HOME, signOut, brand: brandOf(loaded.registry.theme), items });
}

export const onRequest = route({ GET: list });
