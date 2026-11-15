// POST {base}/api/settings/publish — one commit of .ssg.designer.json, for
// designer-ui:settings; keys of the class "executes" or "secret", and any
// value holding a $NAME reference, also need designer-ui:advanced.
//
//   {message, baseBlobId: string | null, changes: [{path: ["mddb", "lang"], value: "en" | null}]}
//   → {commitId, branch, blobId, changed: ["mddb.lang"]}
//
// Order of checks, each refusing the whole publication:
//   1. the site: a YAML config file to sit beside (ssg reads the overlay
//      beside a YAML config only), a readable overlay, baseBlobId equal to
//      the overlay's version on the branch (else 409 — decision 4);
//   2. the next overlay against the schema, exactly as ssg will (422 with
//      every problem, each naming its path, never a value);
//   3. secrets only as references, in the strict form (422);
//   4. permissions: the changed keys that need designer-ui:advanced — 403
//      listing them all when the caller lacks it.
// Each refusal of 2–4 also carries `refused: [{key, reason}]`, one per key.
//   5. the commit: the whole file, sorted, newline at the end; only ever
//      .ssg.designer.json (the settings policy, theme.ts).

import { OVERLAY_FILE } from "../portable/config-files";
import type { Change } from "../portable/repo-provider";
import { OpError, requirePermission } from "../portable/ops";
import type { Op, OpContext } from "../portable/ops";
import { validateOverlay } from "./overlay-validate";
import { applyChanges, parseChanges, serializeOverlay } from "./overlay-write";
import { PERM } from "./permissions";
import { ROOT } from "./schema";
import { changedKeys, needsAdvanced, secretShapeProblems } from "./settings-diff";
import { authorOf } from "../portable/author";
import { siteOf } from "./site";
import type { Site } from "./site";
import { SETTINGS_POLICY, allowedPath } from "./theme";

export interface SettingsPublishInput { message?: unknown; baseBlobId?: unknown; changes?: unknown }
export interface SettingsPublishResult { commitId: string; branch: string; blobId: string; changed: string[] }

export interface SettingsPart { change: Change; path: string; changed: string[] }

// Refused: one key of a settings publication and why it was refused.
export interface Refused { key: string; reason: string }

// refusedOf splits problems written "path: why" (overlay-validate.ts,
// settings-diff.ts — each names its path) into {key, reason}.
export const refusedOf = (problems: readonly string[]): Refused[] =>
  problems.map((p) => {
    const at = p.indexOf(": ");
    return at < 0 ? { key: "", reason: p } : { key: p.slice(0, at), reason: p.slice(at + 2) };
  });

// settingsPart runs checks 1–4 above and builds the overlay's change without
// writing anything — the settings route commits it alone, POST api/publish
// together with theme files (publish.ts). Throws the refusal otherwise.
export function settingsPart(ctx: OpContext, site: Site, input: SettingsPublishInput | undefined): SettingsPart {
  const changes = parseChanges(input?.changes);
  const base = input?.baseBlobId;
  if (base !== null && typeof base !== "string") throw new OpError("invalid_change", 400, "baseBlobId must be the overlay's blobId from GET api/settings, or null when there is none");
  const { state } = site;
  if (state.format !== "yaml") {
    throw new OpError("overlay_needs_yaml", 409, `${OVERLAY_FILE} is read by ssg beside a YAML configuration only; ${state.file ?? "no configuration file"} is found here`);
  }
  if (state.overlayProblem) throw new OpError("overlay_unreadable", 409, `${state.overlayProblem}; fix or remove it in the repository first`);
  if (base !== state.overlayBlobId) {
    throw new OpError("conflict", 409, "the settings changed since they were read", { conflicts: [{ path: OVERLAY_FILE, currentBlobId: state.overlayBlobId, headCommit: state.head }] });
  }
  const before = state.overlay ?? {};
  const after = applyChanges(before, changes);
  const problems = validateOverlay(after);
  if (problems.length) throw new OpError("invalid_settings", 422, `${problems.length} problem(s) in the settings`, { problems, refused: refusedOf(problems) });
  const changed = changedKeys(before, after, state);
  if (!changed.length) throw new OpError("no_change", 400, "the changes leave the settings as they are");
  // Only problems this publication brings: a form written by hand before stays.
  const old = new Set(secretShapeProblems(before, ROOT, ""));
  const shape = secretShapeProblems(after, ROOT, "").filter((p) => !old.has(p));
  if (shape.length) throw new OpError("invalid_settings", 422, "a secret may be written only as a reference", { problems: shape, refused: refusedOf(shape) });
  const advanced = needsAdvanced(changed);
  if (advanced.length) requirePermission(ctx.caller, PERM.advanced, advanced);
  const path = allowedPath(OVERLAY_FILE, SETTINGS_POLICY);
  const change: Change = { op: "put", path, content: { text: serializeOverlay(after) }, baseBlobId: state.overlayBlobId };
  return { change, path, changed: changed.map((c) => c.path) };
}

export const settingsPublishOp: Op<SettingsPublishInput, SettingsPublishResult> = {
  name: "settings_publish",
  permission: PERM.settings,
  run: async (ctx, input) => {
    const site = await siteOf(ctx);
    const part = settingsPart(ctx, site, input);
    const r = await site.repo.provider.commit({ branch: site.branch, message: input?.message as string, author: authorOf(ctx.caller), changes: [part.change] });
    return { commitId: r.commitId, branch: site.branch, blobId: r.blobIds[part.path]!, changed: part.changed };
  },
};
