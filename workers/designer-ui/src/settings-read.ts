// GET {base}/api/settings — the whole configuration, for designer-ui:read
// (README of the round, decision 8: every setting of ssg, grouped, described).
// One entry per key the overlay can address by name (schema.ts namedNodes):
// its schema (type, enum, default, description, group, class, element shape)
// and where its value comes from:
//
//   source "overlay" — .ssg.designer.json sets it (the panel wrote it)
//          "file"    — .ssg.yaml sets it
//          "default" — neither: ssg's default applies
//          "unknown" — the file sets the key it belongs to, but the reader
//                      could not follow it (`unread` says where and why)
//
// Values inside secret keys are {set, reference} (redact.ts); an object's
// value is left to its fields' own entries unless it is open or written in
// its alternate form (`story: true`).

import { effective, lookup } from "./config-read";
import type { Doc } from "../portable/config-files";
import type { ConfigState } from "./config-read";
import type { Op } from "../portable/ops";
import { PERM } from "./permissions";
import { redact } from "./redact";
import { SCHEMA, clsOf, namedNodes } from "./schema";
import type { SchemaGroup, SchemaNode, SettingClass } from "./schema";
import { overlayLeaves } from "./settings-diff";
import { siteOf } from "./site";

export type Source = "overlay" | "file" | "default" | "unknown";

export interface SettingEntry {
  path: string;
  segments: string[]; // the path as keys — what a publication names
  node: Omit<SchemaNode, "fields">; // the schema node; fields are entries of their own
  class: SettingClass; // effective (an element's is its owner's)
  source: Source;
  value?: unknown;
  unread?: string;
}

export interface SettingsView {
  head: string;
  branch: string;
  configFile: string | null;
  configFormat: ConfigState["format"];
  overlayFile: string;
  overlayBlobId: string | null; // the version a publication must name as its base
  overlayKeys: string[]; // the leaf paths the overlay sets
  schemaVersion: number;
  groups: SchemaGroup[];
  entries: SettingEntry[];
  problems: { code: string; detail: string }[];
}

const isObject = (v: unknown): v is Doc => typeof v === "object" && v !== null && !Array.isArray(v);

// entryOf describes one named key in `state`.
function entryOf(state: ConfigState, eff: Doc, node: SchemaNode, segments: string[]): SettingEntry {
  const { fields: _fields, ...shape } = node;
  const entry: SettingEntry = { path: node.path, segments, node: shape, class: clsOf(node), source: "default" };
  const top = segments[0];
  const inOverlay = lookup(state.overlay, segments).has;
  if (inOverlay) entry.source = "overlay";
  else if (top in state.unread) entry.source = "unknown";
  else if (lookup(state.values, segments).has) entry.source = "file";
  if (top in state.unread) entry.unread = `${state.file}: ${top} is set, but its value could not be read (${state.unread[top]})`;
  const v = lookup(eff, segments);
  if (entry.source === "unknown" || !v.has) {
    if (entry.source === "default" && node.default !== undefined) entry.value = redact(node.default, node);
    return entry;
  }
  if (node.type !== "object" || node.open || !isObject(v.value)) entry.value = redact(v.value, node);
  return entry;
}

// problemsOf: what the reader could not do, by code, never with a value.
function problemsOf(state: ConfigState): SettingsView["problems"] {
  const out: SettingsView["problems"] = [];
  if (state.fileProblem) out.push({ code: "config_unreadable", detail: state.fileProblem });
  for (const [k, why] of Object.entries(state.unread)) out.push({ code: "value_unread", detail: `${state.file}: ${k}: ${why}` });
  if (state.include) out.push({ code: "include_not_followed", detail: `${state.file} uses include:; values set in included files are not shown` });
  if (state.overlayProblem) out.push({ code: "overlay_unreadable", detail: state.overlayProblem });
  const known = new Set(SCHEMA.keys.map((k) => k.key));
  for (const k of Object.keys(state.values)) if (!known.has(k) && k !== "include") out.push({ code: "unknown_key", detail: `${state.file}: ${k} is not a configuration key` });
  return out;
}

export function settingsView(state: ConfigState, branch: string): SettingsView {
  const eff = effective(state);
  return {
    head: state.head,
    branch,
    configFile: state.file,
    configFormat: state.format,
    overlayFile: SCHEMA.overlayFile,
    overlayBlobId: state.overlayBlobId,
    overlayKeys: overlayLeaves(state.overlay ?? {}),
    schemaVersion: SCHEMA.schemaVersion,
    groups: SCHEMA.groups,
    entries: namedNodes().map(({ node, segments }) => entryOf(state, eff, node, segments)),
    problems: problemsOf(state),
  };
}

export const settingsReadOp: Op<void, SettingsView> = {
  name: "settings_read",
  permission: PERM.read,
  run: async (ctx) => {
    const { state, branch } = await siteOf(ctx);
    return settingsView(state, branch);
  },
};
