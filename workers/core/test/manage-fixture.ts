// A small site for the module-management tests (GO-149): three modules with
// manifests, one standalone Worker the build could not read, core itself, and
// two templates the site does not use.
import { MODULE } from "../functions/api/core/_module.ts";
import { useRegistry } from "../functions/api/core/_registry.ts";
import { clearStatusCache } from "../functions/api/core/_status.ts";
import { AUTH_ENV, PASSWORD, PW_ENV, basic, call, entry, freshIP, manifest, registry, session } from "./harness.ts";
import { fakeD1 } from "./d1.ts";

export const MANIFESTS = {
  comments: manifest("comments", { nav: [["/comments-admin.html", "comments:read"]], endpoints: { manifest: "/api/comments/module", summary: "/api/comments/module/summary" } }),
  blog: manifest("blog", { nav: [["/blog-admin.html", "blog:read"]], endpoints: { manifest: "/api/blog/module" } }),
  push: manifest("web-push", { perms: ["web-push:notify"], endpoints: {} }),
};
export const SITE = {
  ...registry(entry("blog", MANIFESTS.blog), entry("comments", MANIFESTS.comments), entry("core", MODULE), entry("shop", null, "/shop"), entry("web-push", MANIFESTS.push)),
  available: [
    { id: "contact-form", kind: "pages-function", title: "Contact form" },
    { id: "newsletter-ui", kind: "worker", title: "Newsletter" },
  ],
};
export const IDS = ["blog", "comments", "core", "shop", "web-push"];

export function useSite(): void {
  useRegistry(SITE);
  clearStatusCache();
}

export const CSRF = { "x-core-admin": "1" };
export const pw = (): Record<string, string> => ({ authorization: basic(PASSWORD), "cf-connecting-ip": freshIP() });
export const dbEnv = (db = fakeD1(), extra: Record<string, unknown> = {}) => ({ env: { ...PW_ENV, CORE_DB: db, ...extra }, db });
export const authHeaders = async (perms: string[]): Promise<Record<string, string>> => ({ cookie: await session(perms), accept: "application/json" });
export const authEnv = (db: unknown) => ({ ...AUTH_ENV, CORE_DB: db });

type Handler = Parameters<typeof call>[0];
// send: a state-changing request with the CSRF header and a JSON body.
export const send = (handler: Handler, method: string, path: string, env: Record<string, unknown>, body: unknown, headers: Record<string, string> = pw()) =>
  call(handler, { method, path, env, headers: { ...headers, ...CSRF, "content-type": "application/json" }, body: typeof body === "string" ? body : JSON.stringify(body) });
