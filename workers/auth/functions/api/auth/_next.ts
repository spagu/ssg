// Where a plain HTML form goes after login: the `next` field, accepted only as
// a path on this host — never a scheme, never "//host" (protocol-relative),
// never a backslash (browsers read "/\host" as "//host"), never a control
// character (browsers strip tabs and newlines, so "/\t/host" is "//host").
// Anything else falls back. GO-134's login page relies on this; a login
// endpoint that redirects anywhere it is told is an open redirect.
// A leading underscore keeps this file out of the Pages route table.

const BASE = "https://next.invalid";

// safeNext returns the normalised path, or fallback. Once the string starts
// with exactly one "/" and holds no backslash or control character, the URL
// parser can only read it as a path on BASE — percent-encoded slashes stay
// encoded, inside the path. Its normalised form can still start with "//"
// (dot segments), so that is refused too: the result never names another host.
export function safeNext(raw: unknown, fallback: string): string {
  if (typeof raw !== "string" || raw.length > 512 || !raw.startsWith("/")) return fallback;
  if (raw.startsWith("//") || /[\\\u0000-\u001f\u007f]/.test(raw)) return fallback;
  const url = new URL(raw, BASE);
  // Dot segments collapse during parsing: "/.//evil.example.com" and
  // "/a/..//evil.example.com" come out as "//evil.example.com", which a
  // browser reads as another host. The result is checked again (GO-134).
  const path = url.pathname + url.search + url.hash;
  return path.startsWith("//") ? fallback : path;
}

// withParams adds query parameters to a same-host path (a configured URL such
// as AUTH_LOGIN_URL, or a path safeNext accepted), keeping its own query. An
// absolute configured URL stays absolute.
export function withParams(path: string, params: Record<string, string>): string {
  const url = new URL(path, BASE);
  for (const [k, v] of Object.entries(params)) if (v) url.searchParams.set(k, v);
  return url.origin === BASE ? url.pathname + url.search + url.hash : url.href;
}
