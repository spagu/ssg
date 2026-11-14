// Every request this worker makes to a login provider goes through here
// (GO-151). A leading underscore keeps this file out of the Pages route table.
//
// The guards, each refused BEFORE or INSTEAD of reading anything:
//   - https: only;
//   - the host must be on the provider's hard-coded list — never one taken
//     from a response, a variable or the request;
//   - redirect: "error": a provider endpoint that redirects anywhere (another
//     host included) is a failure, not something to follow;
//   - a time limit and a size limit (the body is read as a stream and cut off
//     past the cap);
//   - JSON only.
// What comes back carries no URL query, header or body text in its `detail`:
// the requests hold codes, verifiers and tokens, and details reach the logs.

export const providerLimits = { timeoutMs: 10_000, maxBytes: 256 * 1024 };

export type Got = { ok: true; status: number; data: unknown } | { ok: false; detail: string };

export interface Call {
  method?: "GET" | "POST";
  headers?: Record<string, string>;
  form?: Record<string, string>; // sent as application/x-www-form-urlencoded
}

// readCapped reads at most `max` bytes, or null past it.
async function readCapped(res: Response, max: number): Promise<Uint8Array | null> {
  if (Number(res.headers.get("content-length")) > max) {
    await res.body?.cancel();
    return null;
  }
  const chunks: Uint8Array[] = [];
  let size = 0;
  if (res.body) {
    const reader = res.body.getReader();
    for (;;) {
      const { done, value } = await reader.read();
      if (done) break;
      size += value.byteLength;
      if (size > max) {
        await reader.cancel();
        return null;
      }
      chunks.push(value);
    }
  }
  const out = new Uint8Array(size);
  let at = 0;
  for (const c of chunks) {
    out.set(c, at);
    at += c.byteLength;
  }
  return out;
}

// providerFetch sends one request under every rule above and parses JSON.
// A non-2xx answer with a JSON body is returned as data (an OAuth error body
// says why); the caller decides.
export async function providerFetch(raw: string, hosts: readonly string[], call: Call = {}): Promise<Got> {
  const url = new URL(raw);
  const where = `${url.host}${url.pathname}`;
  if (url.protocol !== "https:") return { ok: false, detail: `refused ${where}: not https` };
  if (!hosts.includes(url.host)) return { ok: false, detail: `refused ${where}: host not allowed` };
  const headers: Record<string, string> = { accept: "application/json", ...call.headers };
  let body: string | undefined;
  if (call.form) {
    headers["content-type"] = "application/x-www-form-urlencoded";
    body = new URLSearchParams(call.form).toString();
  }
  let res: Response;
  try {
    res = await fetch(url.href, { method: call.method ?? (body ? "POST" : "GET"), headers, body, redirect: "error", signal: AbortSignal.timeout(providerLimits.timeoutMs) });
  } catch (e) {
    const timedOut = e instanceof Error && (e.name === "TimeoutError" || e.name === "AbortError");
    return { ok: false, detail: timedOut ? `${where}: no answer within ${providerLimits.timeoutMs} ms` : `${where}: request failed or redirected` };
  }
  if (res.redirected || (res.status >= 300 && res.status < 400)) {
    await res.body?.cancel();
    return { ok: false, detail: `${where}: redirected` };
  }
  let bytes: Uint8Array | null;
  try {
    bytes = await readCapped(res, providerLimits.maxBytes);
  } catch {
    return { ok: false, detail: `${where}: no answer within ${providerLimits.timeoutMs} ms` };
  }
  if (!bytes) return { ok: false, detail: `${where}: answer larger than ${providerLimits.maxBytes} bytes` };
  try {
    return { ok: true, status: res.status, data: JSON.parse(new TextDecoder().decode(bytes)) as unknown };
  } catch {
    return { ok: false, detail: `${where}: answer is not JSON (status ${res.status})` };
  }
}

// okJson: a 2xx answer whose body is a JSON object, or the reason it is not.
export function okJson(got: Got, what: string): { ok: true; data: Record<string, unknown> } | { ok: false; detail: string } {
  if (!got.ok) return got;
  const d = got.data;
  if (got.status < 200 || got.status >= 300 || !d || typeof d !== "object" || Array.isArray(d)) {
    const code = d && typeof d === "object" && typeof (d as { error?: unknown }).error === "string" ? (d as { error: string }).error.slice(0, 64) : "";
    return { ok: false, detail: `${what}: status ${got.status}${code ? `, error ${code}` : ""}` };
  }
  return { ok: true, data: d as Record<string, unknown> };
}
