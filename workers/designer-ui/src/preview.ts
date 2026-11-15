// GET {base}/api/preview — the site's look with the panel's unpublished
// colour and font tokens (GO-188). The panel shows it in an
// <iframe sandbox>; the response's own headers are set in preview-entry.ts.
//
// Query: path=/blog/post/ (default "/") or page=latest; scheme=light|dark
// (forced on <html data-theme>); lang=en|pl|hi|de|fr (the error page's
// language); and every "--token=value" to apply. Only tokens are previewed:
// templates and settings are what the site was built with.
//
// The answer is always a page: the sanitized site page (200), or a short page
// of our own that says why there is none (4xx/5xx, preview-messages.ts).
// Cost per request: one fetch of the page (plus up to three same-origin
// redirects); page=latest adds the home page and its feed. Each fetch is
// capped by the plan (PAGE_LIMIT) and by FETCH_TIMEOUT (preview-fetch.ts).

import type { Op } from "../portable/ops";
import { basePath } from "./http";
import { PERM } from "./permissions";
import { PAGE_LIMIT, PreviewError, fetchPage, sitePath } from "./preview-fetch";
import { activePlan } from "../portable/plan";
import type { FetchFn } from "../portable/repo-github-http";
import { latestPath } from "./preview-latest";
import { errorPage, langOf } from "./preview-messages";
import { sanitize } from "./preview-sanitize";
import { tokensCSS, tokensFrom } from "./preview-tokens";

export interface PreviewOut { status: number; html: string }

const schemeOf = (v: string | null): "light" | "dark" | null => (v === "light" || v === "dark" ? v : null);

export const previewOp: Op<URLSearchParams, PreviewOut> = {
  name: "preview",
  permission: PERM.read,
  run: async (ctx, q) => {
    const doFetch: FetchFn = ctx.deps.fetch ?? ((url, init) => fetch(url, init));
    const base = basePath(ctx.env)!; // the pipeline answered 503 before any route when it is not a path
    const origin = ctx.url.origin;
    try {
      const scheme = schemeOf(q.get("scheme"));
      const css = tokensCSS(tokensFrom(q), scheme);
      const cap = PAGE_LIMIT[activePlan(ctx.env).plan];
      const path = q.get("page") === "latest" ? await latestPath(doFetch, origin, base, cap) : sitePath(q.get("path"), base);
      const page = await fetchPage(doFetch, origin, path, cap);
      return { status: 200, html: sanitize(page.text, { baseHref: page.url, scheme, css }) };
    } catch (err) {
      if (err instanceof PreviewError) return { status: err.status, html: errorPage(langOf(q.get("lang")), err.code) };
      throw err;
    }
  },
};

export const previewResponse = (out: PreviewOut): Response =>
  new Response(out.html, { status: out.status, headers: { "content-type": "text/html; charset=utf-8" } });
