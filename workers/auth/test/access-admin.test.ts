// GO-139: the admin API of the Access group mapping —
// /api/auth/admin/access-groups (auth:admin).
import { test } from "node:test";
import assert from "node:assert/strict";
import { onRequestGet as list, onRequestPost as create } from "../functions/api/auth/admin/access-groups/index.ts";
import { onRequestDelete as remove } from "../functions/api/auth/admin/access-groups/[id].ts";
import { addGrant } from "../functions/api/auth/_grants.ts";
import { createGroup, deleteGroup } from "../functions/api/auth/_groups.ts";
import type { Group } from "../functions/api/auth/_groups.ts";
import { T0, call, useClock } from "./fakes.ts";
import { roleId, signedIn } from "./seed.ts";
import { providerUser } from "./go151.ts";
import { FakeAccess, PERSON, accessEnv, enter, mint } from "./access-fake.ts";

test("access-groups: list, create (applied at once to people already seen), refuse bad input, escalation and duplicates, delete", async () => {
  useClock();
  const env = accessEnv();
  const fake = new FakeAccess().install();
  try {
    const admin = await signedIn(env, "admin@example.com", ["auth:admin", "newsletter:read"]);
    const viewers = (await createGroup(env, "Viewers", "", T0)) as Group;
    await addGrant(env, roleId(env as never, "newsletter-viewer"), { groupId: viewers.id }, "test", T0);
    const editors = (await createGroup(env, "Editors", "", T0)) as Group;
    await addGrant(env, roleId(env as never, "newsletter-editor"), { groupId: editors.id }, "test", T0);
    // Someone signed in through Access with the group "readers" before any mapping.
    const ada = await providerUser(env, "ada@example.com", "access", PERSON.sub);
    await enter(env, await mint({ ...PERSON, groups: ["readers"] }));

    const post = (json: unknown) => call(create, env, "/api/auth/admin/access-groups", { json, browser: admin.browser });
    const made = await post({ accessGroup: " readers ", groupId: viewers.id });
    assert.equal(made.status, 201);
    const body = await made.json();
    assert.deepEqual({ ...body.mapping, id: undefined, createdAt: undefined }, { id: undefined, accessGroup: "readers", groupId: viewers.id, groupName: "Viewers", createdBy: admin.id, createdAt: undefined });
    assert.equal(body.resynced, 1);
    assert.equal(env.AUTH_DB.sqlite.prepare("SELECT COUNT(*) AS n FROM group_members WHERE user_id = ?").get(ada).n, 1);

    for (const [json, status, error] of [
      [{ accessGroup: "", groupId: viewers.id }, 400, "invalid_request"],
      [{ accessGroup: "x\u0001", groupId: viewers.id }, 400, "invalid_request"],
      [{ accessGroup: "x".repeat(257), groupId: viewers.id }, 400, "invalid_request"],
      [{ accessGroup: 5, groupId: viewers.id }, 400, "invalid_request"],
      [{ accessGroup: "x" }, 400, "invalid_request"],
      [{ accessGroup: "x", groupId: "nope" }, 404, "group_not_found"],
      [{ accessGroup: "readers", groupId: viewers.id }, 409, "mapping_exists"],
      [{ accessGroup: "writers", groupId: editors.id }, 403, "escalation"],
    ] as const) {
      const res = await post(json);
      assert.deepEqual([res.status, (await res.json()).error], [status, error], JSON.stringify(json));
    }
    const notJson = await call(create, env, "/api/auth/admin/access-groups", { method: "POST", browser: admin.browser, headers: { "content-type": "application/json" } });
    assert.equal(notJson.status, 400);

    const listed = await (await call(list, env, "/api/auth/admin/access-groups", { browser: admin.browser })).json();
    assert.equal(listed.items.length, 1);
    assert.equal(listed.nextCursor, null);
    await deleteGroup(env, viewers.id);
    const again = await (await call(list, env, "/api/auth/admin/access-groups", { browser: admin.browser })).json();
    assert.equal(again.items[0].groupName, null, "a mapping to a deleted group shows it and does nothing");

    const del = (id: string) => call(remove, env, `/api/auth/admin/access-groups/${id}`, { method: "DELETE", browser: admin.browser, params: { id } });
    const gone = await del(listed.items[0].id);
    assert.deepEqual(await gone.json(), { ok: true, resynced: 0 });
    const missing = await del(listed.items[0].id);
    assert.deepEqual([missing.status, await missing.json()], [404, { error: "mapping_not_found" }]);
  } finally {
    fake.restore();
  }
});
