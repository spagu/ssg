// Cloudflare Pages Function: the site's basics, in one place.
//   GET /api/core/info[?lang=pl] →
//     {checkedAt, mode, user, lang,
//      registry: {format, modules},          the build's module registry
//      modules: [{id, title, kind, version, contract, status}],
//      problems: [{module, title, code, text}],   from the modules' summaries
//      build: BuildFacts | null, buildState: ok | absent | unreadable | invalid,
//      buildOption: "build_report",
//      theme: Theme | null}                    admin_theme: as built (_theme.ts)
//
// Nothing here is secret and nothing comes from core's environment: the mode
// is a word, versions and contracts come from the manifests, problems are the
// modules' own sentences (which name what is missing, never a value), and the
// build facts are the site's public build.json (_build.ts). Visibility follows
// /api/core/modules; core itself is listed (its version matters).

import { problemsOf } from "./_attention";
import { BUILD_OPTION, buildJob, buildView } from "./_build";
import { json } from "./_lib";
import type { Env } from "./_lib";
import { gather } from "./_overview";
import { loadRegistry } from "./_registry";
import { route } from "./_route";
import type { Caller } from "./_signin";
import { ask } from "./_status";
import { pickLang } from "./_view";

async function info(request: Request, env: Env, caller: Caller): Promise<Response> {
  const loaded = await loadRegistry();
  if (!loaded.ok) return loaded.response;
  const lang = pickLang(request);
  const started = Date.now();
  const [gathered, build] = await Promise.all([gather(request, env, caller, lang, loaded.registry, started), ask(request, env, [buildJob(env)], started)]);
  const modules = gathered.map((g) => ({
    id: g.entry.id,
    title: g.view.title,
    kind: g.view.kind,
    version: g.view.version,
    contract: g.manifest?.contract ?? null,
    status: g.view.status,
  }));
  const problems = problemsOf(
    gathered.map((g) => ({ id: g.entry.id, title: g.view.title, base: g.entry.base, summary: g.accepted })),
    lang,
  );
  return json({
    checkedAt: new Date().toISOString(),
    mode: caller.mode,
    user: caller.user,
    lang,
    registry: { format: loaded.registry.format, modules: loaded.registry.modules.length },
    modules,
    problems,
    ...buildView(build.get("build")!),
    buildOption: BUILD_OPTION,
    theme: loaded.registry.theme,
  });
}

export const onRequest = route({ GET: info });
