// Shared by the tests: a configured worker, one call's whole answer
// (status, sorted headers, body bytes), and a GitHub stand-in.
import { onRequestPost } from "../functions/api/republish/index.ts";

export const BASE = {
  REPUBLISH_KEY: "k3y-for-tests",
  REPUBLISH_PROVIDER: "github",
  REPUBLISH_TOKEN: "ghp_provider_token",
  REPUBLISH_REPO: "owner/site",
  REPUBLISH_WORKFLOW: "deploy.yml",
};

type Handler = typeof onRequestPost;
export async function answer(handler: Handler, env: Record<string, unknown>, init: { url?: string; headers?: Record<string, string>; method?: string } = {}) {
  const request = new Request(init.url ?? "https://example.com/api/republish", { method: init.method ?? "POST", headers: init.headers ?? {} });
  const res = await handler({ request, env, params: {}, data: {}, waitUntil: () => {}, next: async () => new Response() } as never);
  return [res.status, [...res.headers].sort(), await res.text()];
}

// github: the dispatch stand-in answers 204 (accepted) and records the calls.
export function github(t: { mock: { method: (o: object, n: string, f: unknown) => unknown } }, status = 204) {
  const calls: string[] = [];
  t.mock.method(globalThis, "fetch", async (url: string) => (calls.push(url), new Response(status === 204 ? null : "nope", { status })));
  return calls;
}
