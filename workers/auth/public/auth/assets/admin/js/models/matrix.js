// The access matrix: accounts × modules — for each, whether the account
// holds none, some or all of the module's known permissions — and the text
// summary under it. Pure functions over GET /api/auth/admin/matrix and the
// registry (models/perms.js modulesOf).

import { can } from "./access.js";

export const LEVELS = ["all", "some", "none"];

// cellOf: how much of one module an account's permissions reach.
export function cellOf(perms, module) {
  const total = module.permissions.length;
  const held = module.permissions.filter((p) => can(perms, p.name)).length;
  return { module: module.id, held, total, level: held === 0 ? "none" : held === total ? "all" : "some" };
}

// rowsOf: one row per account, one cell per module (in the modules' order).
export const rowsOf = (users, modules) => users.map((u) => ({ user: u, cells: modules.map((m) => cellOf(u.permissions, m)) }));

// tally: per module, how many accounts hold all, some or none of it.
export function tally(rows, modules) {
  return modules.map((m, i) => {
    const out = { module: m.id, all: 0, some: 0, none: 0 };
    for (const r of rows) out[r.cells[i].level] += 1;
    return out;
  });
}

// filterRows keeps the rows whose cell in `module` has `level` ("" = any
// module or level; level "any" = all or some).
export function filterRows(rows, modules, { module = "", level = "" } = {}) {
  if (!module || !level) return rows;
  const i = modules.findIndex((m) => m.id === module);
  if (i < 0) return rows;
  return rows.filter((r) => (level === "any" ? r.cells[i].level !== "none" : r.cells[i].level === level));
}
