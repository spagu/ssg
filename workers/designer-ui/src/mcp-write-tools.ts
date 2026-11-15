// The MCP tools that write: each call is ONE publication — one commit with
// every file or key in it and the caller's `message` — so an assistant never
// makes a commit per key or per file. The input sent to the operation is the
// body its REST route takes (POST api/theme/publish, api/settings/publish,
// api/publish), so both refuse alike: permissions (designer-ui:advanced per
// key), the theme's path policy, the schema, secrets only as references,
// versions and conflicts.

import { overlayVersionOp } from "./config-key";
import { invalid, items, keyOf, list, part, s, schema } from "./mcp-args";
import type { DesignerTool } from "./mcp-args";
import { PERM } from "./permissions";
import { publishOp } from "./publish";
import { settingsPublishOp } from "./settings-publish";
import { themePublishOp } from "./theme-publish";
import type { Call, Json } from "../portable/mcp-transport";
import type { Op } from "./ops";

const MESSAGE = s("The commit message: what changed and why. First line at most 200 characters");
const ONE_COMMIT = " One call = one commit: give several entries in `changes` rather than one call each.";
const WRITE = { readOnlyHint: false, destructiveHint: false, idempotentHint: false, openWorldHint: false };
const nullable = (description: string): Json => ({ type: ["string", "null"], description });

// A theme file to write, and a setting to change.
const FILE: Json = {
  path: s("Repository path inside the theme folder, e.g. templates/mytheme/css/style.css"),
  content: s("The complete new text of the file (not a patch)"),
  content_base64: s("Instead of content: the bytes, base64-encoded (images, fonts)"),
  baseBlobId: nullable("The blobId designer_read or designer_list gave; null for a new file. Refused as conflict when the file moved on"),
};
const FILE_KEYS = Object.keys(FILE);
const SETTING: Json = {
  key: { description: 'The setting, dotted ("mddb.lang") or as a list of keys (from designer_config_read)', oneOf: [{ type: "string" }, { type: "array", items: { type: "string" } }] },
  value: { description: "The new value as JSON (true, 3, \"text\", [\"a\"], {\"k\": 1}); null removes the key from the settings file" },
};
const BASE = nullable("overlayBlobId from designer_config_read; omitted = the settings file as it is now (a key changed meanwhile is overwritten)");

// many: the schema of a write — the entries as `changes`, or one at the top level.
function many(entry: Json, required: string[], extra: Json = {}): Json {
  const item = schema(entry, ...required);
  return schema({ message: MESSAGE, ...extra, ...entry, changes: { type: "array", minItems: 1, items: item, description: "Several entries in the same commit" } }, "message");
}

// themeFiles: the theme files as the theme route's `put` changes.
const themeFiles = (files: Json[]): Json[] =>
  files.map((c) => ({ op: "put", path: c.path, content: c.content_base64 === undefined ? { text: c.content } : { base64: c.content_base64 }, baseBlobId: c.baseBlobId }));

// settingsInput: the settings route's {baseBlobId, changes}. A missing
// baseBlobId is the overlay's version now (overlayVersionOp, read permission).
async function settingsInput(base: unknown, entries: Json[], call: Call<Op>, at: string): Promise<Json> {
  const changes = entries.map((c, i) => {
    const where = `${at}[${i}].key`;
    const path = keyOf(c, "key", where);
    if (!path) throw invalid(`${where} is required`);
    return "value" in c ? { path, value: c.value } : { path };
  });
  const baseBlobId = base === undefined ? ((await call(overlayVersionOp, undefined)) as { overlayBlobId: string | null }).overlayBlobId : base;
  return { baseBlobId, changes };
}

export const WRITE_TOOLS: DesignerTool[] = [
  {
    name: "designer_write",
    description: "DESIGNER: Create or replace theme files with their complete new content (templates, styles, scripts, " +
      "images, fonts), each with its baseBlobId. Only paths inside the theme folder with a theme extension; never the " +
      "configuration. A template is code the build runs and every visitor gets: this needs designer-ui:theme." + ONE_COMMIT,
    inputSchema: many(FILE, ["path", "baseBlobId"]),
    annotations: WRITE,
    op: themePublishOp,
    input: (a) => ({ message: a.message, changes: themeFiles(items(a, FILE_KEYS)) }),
  },
  {
    name: "designer_config_set",
    description: "DESIGNER: Change settings: each `key` (as designer_config_read names it) gets `value`; null removes it " +
      "from .ssg.designer.json. Checked as ssg checks it; refused whole, with `refused: [{key, reason}]`, for an unknown " +
      "key, a wrong type, a secret written as a value instead of a \"$NAME\" reference, or a key that needs " +
      "designer-ui:advanced (code, deployment, any $NAME)." + ONE_COMMIT,
    inputSchema: many(SETTING, ["key", "value"], { baseBlobId: BASE }),
    annotations: WRITE,
    op: settingsPublishOp,
    input: async (a, call) => ({ message: a.message, ...(await settingsInput(a.baseBlobId, items(a, ["key", "value"]), call, "changes")) }),
  },
  {
    name: "designer_publish",
    description: "DESIGNER: Settings and theme files in ONE commit, when they belong together (a new template and the " +
      "setting that uses it): `settings` as designer_config_set takes them, `theme` as designer_write. Every check of " +
      "both runs before anything is written; a refusal refuses the whole publication.",
    inputSchema: schema({
      message: MESSAGE,
      settings: schema({ baseBlobId: BASE, changes: { type: "array", minItems: 1, items: schema(SETTING, "key", "value") } }, "changes"),
      theme: schema({ changes: { type: "array", minItems: 1, items: schema(FILE, "path", "baseBlobId") } }, "changes"),
    }, "message"),
    annotations: WRITE,
    permissions: [PERM.settings, PERM.theme],
    op: publishOp,
    input: async (a, call) => {
      const settings = part(a, "settings");
      const theme = part(a, "theme");
      return {
        message: a.message,
        ...(settings ? { settings: await settingsInput(settings.baseBlobId, list(settings.changes, "settings.changes"), call, "settings.changes") } : {}),
        ...(theme ? { theme: { changes: themeFiles(list(theme.changes, "theme.changes")) } } : {}),
      };
    },
  },
];
