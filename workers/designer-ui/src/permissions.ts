// The permissions of designer-ui — module.json's list, the one the `auth`
// worker shows when an owner builds a role. Code names them through PERM, so
// a rename is one line here plus module.json (a test keeps the two equal).
//
// The namespace is the module's id, as docs/WORKER-MODULES.md requires of a
// declaration (workers/modules.test.ts checks it).
//
// `advanced` and `theme` are sensitive (README of the round, "Uprawnienia"):
// whoever holds them has the trust of a person with write access to the
// repository. module.json declares them `"sensitive": true`, and the `auth`
// worker reads that from its registry beside its name rules (GO-181,
// workers/auth/functions/api/auth/_service-perms.ts): a service token carries
// them only with an owner's allowSensitive.

import manifest from "../module.json" with { type: "json" };
import { can } from "./session-verifier";

export const PERM = {
  read: "designer-ui:read", // theme files, every setting (secrets as set / not set), revisions
  settings: "designer-ui:settings", // publish settings of the class "plain"
  advanced: "designer-ui:advanced", // settings that run code or deploy, and secret references
  theme: "designer-ui:theme", // publish theme files
} as const;

export type Permission = (typeof PERM)[keyof typeof PERM];

// DECLARED: the permissions module.json declares, in its order.
export const DECLARED: readonly string[] = manifest.permissions.map((p) => p.name);

// held: the declared permissions `perms` grants (wildcards expanded through
// the verifier's own `can`, other modules' permissions left out).
export const held = (perms: readonly string[]): string[] => DECLARED.filter((p) => can(perms, p));
