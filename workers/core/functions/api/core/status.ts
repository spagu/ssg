// Cloudflare Pages Function: how each module is doing, asked live.
//   GET /api/core/status → {checkedAt, items: [{id, status, version, ms,
//                           httpStatus?, reason?, detail?, cached?}]}
//
// status, per module:
//   reachable         it answered its health (or manifest) endpoint
//   unreachable       no answer in time, a network error, an error status, a
//                     redirect (never followed), or a URL off this host
//                     (reason says which)
//   denied            it refused the caller's credential (401/403)
//   invalid_manifest  its manifest — from the build or asked live — is refused
//                     by the reader, or larger than the cap
//   unknown           it declares no endpoint to ask
// Requests go out in parallel, each with a timeout, and the answer comes
// within an overall deadline; answers are cached briefly per caller
// (_status.ts). Only modules this caller may see are listed (_view.ts).
//
// With CORE_DB, each answer that flips a module between reachable and
// unreachable is recorded (_history.ts) — on observation only: the home page
// asks this route when it is opened or refreshed, never on a timer.

import { json } from "./_lib";
import type { Env } from "./_lib";
import { loadRegistry } from "./_registry";
import { route } from "./_route";
import type { Caller } from "./_signin";
import { observe } from "./_history";
import { askAll } from "./_status";
import type { Answer } from "./_status";
import { registryManifest, visibleTo } from "./_view";

async function status(request: Request, env: Env, caller: Caller): Promise<Response> {
  const loaded = await loadRegistry();
  if (!loaded.ok) return loaded.response;
  const known = loaded.registry.modules.map((entry) => ({ entry, ...registryManifest(entry) }));
  // Ask only modules this caller may see. A module whose manifest the build
  // already knows is filtered before any request goes out — asking it would
  // send this caller's credential to a module it cannot use and earn a 403
  // in that module's logs. A module the build knows nothing about must be
  // asked to learn its manifest, and is filtered afterwards.
  const asked = known.filter((k) => k.problem === null && (k.manifest === null || visibleTo(k.manifest, caller.perms)));
  const answers = await askAll(request, env, asked.map((k) => ({ entry: k.entry, manifest: k.manifest })));

  const items: (Answer & { id: string; version: string | null })[] = [];
  for (const k of known) {
    const answer: Answer = answers.get(k.entry.id) ?? { status: "invalid_manifest", ms: null, reason: "registry", detail: k.problem! };
    const manifest = k.manifest ?? answer.manifest ?? null;
    if (!visibleTo(manifest, caller.perms)) continue;
    const { manifest: _omit, ...shown } = answer;
    items.push({ id: k.entry.id, version: answer.manifest?.version ?? manifest?.version ?? k.entry.version, ...shown });
  }
  await observe(env, items);
  return json({ checkedAt: new Date().toISOString(), items });
}

export const onRequest = route({ GET: status });
