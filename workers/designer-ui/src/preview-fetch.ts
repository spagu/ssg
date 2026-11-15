// Fetching the page to preview (GO-188): a public page of the panel's own
// host, the way an anonymous visitor gets it.
//   - Nothing of the caller travels: a new request with no cookie, no
//     Authorization, no header of the incoming request.
//   - The host never changes: a redirect is followed only to the same origin
//     (a site redirects /blog to /blog/), at most MAX_HOPS times; one that
//     leaves the host is refused, never followed.
//   - The page is HTML and at most the plan's PAGE_LIMIT bytes, counted as
//     they arrive (SR-24: Free's 10 ms of CPU sanitizes about 256 KiB).
//   - One fetch, its redirects and its body together take at most
//     FETCH_TIMEOUT (SR-25); then upstream_timeout (504).
// Nothing of the answer but the HTML is used: its cookies and headers stay here.

import type { Plan } from "../portable/plan";
import type { FetchFn } from "../portable/repo-github-http";

export const MAX_PAGE = 2 * 1024 * 1024;
export const PAGE_LIMIT: Record<Plan, number> = { free: 256 * 1024, paid: MAX_PAGE };
export const MAX_HOPS = 3;
export const FETCH_TIMEOUT = 10_000;

export class PreviewError extends Error {
  readonly code: string; // a key of preview-messages.ts
  readonly status: number;
  constructor(code: string, status: number) {
    super(code);
    this.name = "PreviewError";
    this.code = code;
    this.status = status;
  }
}

// sitePath: a path the caller asked for, checked as a path — never a URL:
// "/" then path characters, no "//", no "\\", no dot segment, no control
// character, not inside the worker's own base path. A query is allowed.
export function sitePath(raw: string | null, base: string): string {
  const path = raw ?? "/";
  const [pathname] = path.split("?", 1);
  const ok =
    path.length <= 2048 &&
    /^\/[A-Za-z0-9\-._~!$&'()*+,;=:@%/?]*$/.test(path) &&
    !pathname.startsWith("//") &&
    !/(^|\/)(\.|%2e){1,2}(\/|$)/i.test(pathname) &&
    !(base && (pathname === base || pathname.startsWith(base + "/")));
  if (!ok) throw new PreviewError("bad_path", 400);
  return path;
}

// KINDS: what a page and a feed are, by Accept and by Content-Type.
const KINDS = {
  html: { accept: "text/html", type: /^text\/html/i },
  feed: { accept: "application/feed+json, application/atom+xml, application/rss+xml, application/xml, text/xml", type: /^(application\/(feed\+json|json|atom\+xml|rss\+xml|xml)|text\/xml)/i },
};

// fetchPage returns the text at `path` on `origin` (at most maxBytes) and the
// address it came from (after same-origin redirects).
export async function fetchPage(doFetch: FetchFn, origin: string, path: string, maxBytes = MAX_PAGE, kind: keyof typeof KINDS = "html"): Promise<{ url: string; text: string }> {
  const limit = deadline(FETCH_TIMEOUT);
  try {
    return await follow(doFetch, origin, path, maxBytes, kind, limit);
  } finally {
    limit.clear();
  }
}

async function follow(doFetch: FetchFn, origin: string, path: string, maxBytes: number, kind: keyof typeof KINDS, limit: Deadline): Promise<{ url: string; text: string }> {
  const { accept, type } = KINDS[kind];
  let url = new URL(path, origin);
  for (let hop = 0; ; hop++) {
    const sent = doFetch(url.href, { method: "GET", redirect: "manual", signal: limit.signal, headers: { accept, "user-agent": "ssg-designer-ui-preview" } }).catch(() => {
      throw new PreviewError("page_unavailable", 502); // the network error's text stays here
    });
    const res = await limit.race(sent);
    if (res.status >= 300 && res.status < 400) {
      await res.body?.cancel();
      const next = URL.parse(res.headers.get("location") ?? "", url); // null: not an address at all
      if (!next) throw new PreviewError("page_unavailable", 502);
      if (next.origin !== origin) throw new PreviewError("redirect_off_host", 502);
      if (hop >= MAX_HOPS) throw new PreviewError("too_many_redirects", 502);
      url = next;
      continue;
    }
    if (!res.ok) {
      await res.body?.cancel();
      throw new PreviewError(res.status === 404 ? "page_not_found" : "page_unavailable", res.status === 404 ? 404 : 502);
    }
    if (!type.test(res.headers.get("content-type") ?? "")) {
      await res.body?.cancel();
      throw new PreviewError("not_html", 415);
    }
    return { url: url.href, text: await readCapped(res, maxBytes, limit) };
  }
}

// readCapped: the body as UTF-8 text, refused past maxBytes bytes.
async function readCapped(res: Response, maxBytes: number, limit: Deadline): Promise<string> {
  if (Number(res.headers.get("content-length")) > maxBytes) {
    await res.body?.cancel();
    throw new PreviewError("page_too_large", 413);
  }
  if (!res.body) return "";
  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  let size = 0;
  let text = "";
  for (let r = await limit.race(reader.read()); !r.done; r = await limit.race(reader.read())) {
    size += r.value.byteLength;
    if (size > maxBytes) {
      await reader.cancel();
      throw new PreviewError("page_too_large", 413);
    }
    text += decoder.decode(r.value, { stream: true });
  }
  return text + decoder.decode();
}

// Deadline: one fetch's time limit. race() rejects with upstream_timeout once
// it passes, whether or not the fetch honours `signal` (aborted at the same
// moment, so a Worker stops the subrequest too).
interface Deadline { signal: AbortSignal; race: <T>(p: Promise<T>) => Promise<T>; clear: () => void }

function deadline(ms: number): Deadline {
  const ctrl = new AbortController();
  let timer!: ReturnType<typeof setTimeout>; // set at once by the executor below
  const expired = new Promise<never>((_, reject) => {
    timer = setTimeout(() => {
      reject(new PreviewError("upstream_timeout", 504));
      ctrl.abort();
    }, ms);
  });
  expired.catch(() => {}); // rejected while nothing waits on it: not an unhandled rejection
  return { signal: ctrl.signal, race: (p) => Promise.race([p, expired]), clear: () => clearTimeout(timer) };
}
