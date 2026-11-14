// Reading a state-changing request: the CSRF defence and the body. A leading
// underscore keeps this file out of the Pages route table.
//
// CSRF. Every POST must pass BOTH:
//   1. no cross-origin evidence: an Origin header, when sent, is this origin
//      (never "null"), and Sec-Fetch-Site, when sent, is "same-origin" (a
//      sibling subdomain is "same-site" and is refused: it may be someone
//      else's);
//   2. positive proof it came from us, one of:
//      - the custom header `x-ssg-auth: 1` (scripts, fetch). A page on another
//        origin cannot add it without a CORS preflight, and this worker never
//        answers one, so the browser never sends the request;
//      - for a plain HTML form (which cannot set headers), on routes that accept
//        one: an Origin equal to this origin, or Sec-Fetch-Site: same-origin.
//        Every current browser sends Origin on a form POST; one that sends
//        neither header is refused — it can still use the JSON API.
// SameSite does the rest: the access cookie is Lax (not sent on a cross-site
// POST) and the refresh cookie Strict. The form path matters mostly for
// /login, where the attack is "log the victim into the attacker's account"
// (login CSRF), which cookies alone cannot stop — the Origin proof does.

export const CSRF_HEADER = "x-ssg-auth";
const MAX_BODY = 16 * 1024;

export function isForm(request: Request): boolean {
  const type = (request.headers.get("content-type") || "").toLowerCase();
  return type.startsWith("application/x-www-form-urlencoded") || type.startsWith("multipart/form-data");
}

// csrfProblem returns why the request is refused, or null.
export function csrfProblem(request: Request, allowForm: boolean): string | null {
  const own = new URL(request.url).origin;
  const origin = request.headers.get("origin");
  const site = request.headers.get("sec-fetch-site");
  if (origin !== null && origin !== own) return "cross_origin";
  if (site !== null && site !== "same-origin") return "cross_origin";
  if (request.headers.get(CSRF_HEADER) === "1") return null;
  if (allowForm && isForm(request) && (origin === own || site === "same-origin")) return null;
  return "csrf_check_failed";
}

export type Body = Record<string, unknown>;

// readBody parses a JSON object or an HTML form (urlencoded or multipart) of at
// most 16 KiB, or returns null. A form's fields arrive as strings; files are
// dropped.
export async function readBody(request: Request): Promise<Body | null> {
  const declared = Number(request.headers.get("content-length") || "0");
  if (declared > MAX_BODY) return null;
  const raw = await request.text();
  if (raw.length > MAX_BODY) return null;
  if (isForm(request)) {
    try {
      const form = await new Response(raw, { headers: { "content-type": request.headers.get("content-type") as string } }).formData();
      const out: Body = {};
      for (const [k, v] of form) if (typeof v === "string") out[k] = v;
      return out;
    } catch {
      return null;
    }
  }
  try {
    const v = JSON.parse(raw);
    return v && typeof v === "object" && !Array.isArray(v) ? (v as Body) : null;
  } catch {
    return null;
  }
}

export const str = (v: unknown): string => (typeof v === "string" ? v : "");
