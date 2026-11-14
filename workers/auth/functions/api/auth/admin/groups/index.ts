// Cloudflare Pages Function: groups.
//   GET  /api/auth/admin/groups   → 200 {items: [group], nextCursor: null}
//   POST /api/auth/admin/groups   {name, description?} → 201 {group}
//
// Requires `auth:admin`. A group is {id, name, description, memberCount,
// createdAt, updatedAt}; `name` is free text, 1–100 characters, unique. 400
// invalid_request {detail}; 409 name_taken.

import { authorize, body, invalid, nameTaken, text } from "../../_admin";
import { GROUP_NAME_RULE, createGroup, groupName, listGroups } from "../../_groups";
import { json } from "../../_lib";
import type { Env } from "../../_lib";

export const onRequestGet: PagesFunction<Env> = async ({ request, env }) => {
  const caller = await authorize(request, env, "auth:admin");
  if (caller instanceof Response) return caller;
  return json({ items: await listGroups(env), nextCursor: null });
};

export const onRequestPost: PagesFunction<Env> = async ({ request, env }) => {
  const caller = await authorize(request, env, "auth:admin");
  if (caller instanceof Response) return caller;
  const b = await body(request);
  if (b instanceof Response) return b;
  const name = groupName(b.name);
  if (!name) return invalid(GROUP_NAME_RULE);
  const description = text(b, "description", 500);
  if (description === null) return invalid("description: a string of at most 500 characters");
  const group = await createGroup(env, name, description ?? "", caller.ctx.now);
  if (group === "name_taken") return nameTaken();
  return json({ group }, 201);
};
