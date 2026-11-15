// The settings as the MCP tool designer_config_read gives them (GO-181), for
// designer-ui:read. Without a key: exactly GET api/settings — every setting
// with its schema, value and source, secrets as {set, reference} — so an
// assistant sees what the panel shows. With a key: that key's entry and the
// entries of its fields (an object key), plus what a publication needs
// (overlayBlobId). A key the panel does not list is refused naming it.

import { OpError } from "../portable/ops";
import type { Op } from "../portable/ops";
import { PERM } from "./permissions";
import { settingsView } from "./settings-read";
import type { SettingEntry, SettingsView } from "./settings-read";
import { siteOf } from "./site";

export interface ConfigKeyView {
  head: string;
  branch: string;
  configFile: string | null;
  overlayFile: string;
  overlayBlobId: string | null;
  key: string;
  entries: SettingEntry[];
  problems: SettingsView["problems"];
}

// under: is `segments` the key `key` or one of its fields?
const under = (segments: readonly string[], key: readonly string[]): boolean =>
  segments.length >= key.length && key.every((k, i) => segments[i] === k);

export const configReadOp: Op<{ key: string[] | null } | undefined, SettingsView | ConfigKeyView> = {
  name: "designer_config_read",
  permission: PERM.read,
  run: async (ctx, input) => {
    const { state, branch } = await siteOf(ctx);
    const view = settingsView(state, branch);
    const key = input?.key;
    if (!key) return view;
    const name = key.join(".");
    const entries = view.entries.filter((e) => under(e.segments, key));
    if (!entries.length) {
      throw new OpError("unknown_key", 404, `${name} is not a setting the designer lists; call designer_config_read without a key for every one`, {
        refused: [{ key: name, reason: "unknown configuration key" }],
      });
    }
    const { head, configFile, overlayFile, overlayBlobId, problems } = view;
    return { head, branch, configFile, overlayFile, overlayBlobId, key: name, entries, problems };
  },
};

// overlayVersionOp: the overlay's version on the branch, for a settings
// publication from MCP whose caller did not name one (mcp-write-tools.ts).
export const overlayVersionOp: Op<void, { overlayBlobId: string | null }> = {
  name: "overlay_version",
  permission: PERM.read,
  run: async (ctx) => ({ overlayBlobId: (await siteOf(ctx)).state.overlayBlobId }),
};
