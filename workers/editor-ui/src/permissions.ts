// The permissions of editor-ui — module.json's list, the one the `auth`
// worker shows when an owner builds a role. Code names them through PERM, so
// a rename is one line here plus module.json (a test keeps the two equal).
//
// The namespace is the module's id, as docs/WORKER-MODULES.md requires of a
// declaration (workers/modules.test.ts checks it).

import manifest from "../module.json" with { type: "json" };
import { can } from "./session-verifier";

export const PERM = {
  read: "editor-ui:read", // list and read content, revisions, the media library
  publish: "editor-ui:publish", // commit content changes: create, edit, rename
  media: "editor-ui:media", // add images in a publication
  delete: "editor-ui:delete", // remove a content file or an image
} as const;

export type Permission = (typeof PERM)[keyof typeof PERM];

// DECLARED: the permissions module.json declares, in its order.
export const DECLARED: readonly string[] = manifest.permissions.map((p) => p.name);

// held: the declared permissions `perms` grants (wildcards expanded through
// the verifier's own `can`, other modules' permissions left out).
export const held = (perms: readonly string[]): string[] => DECLARED.filter((p) => can(perms, p));
