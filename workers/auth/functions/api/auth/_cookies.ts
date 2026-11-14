// The two cookies of a session. A leading underscore keeps this file out of
// the Pages route table.
//
//   __Host-ssg-auth          the access token (a short JWT). The __Host- prefix
//                            makes the browser refuse it unless it is Secure,
//                            has Path=/ and no Domain: it belongs to this exact
//                            host and every worker on the host can read it.
//                            SameSite=Lax, so following a link to a panel from
//                            elsewhere arrives logged in.
//   __Secure-ssg-auth-refresh the opaque refresh token, sent ONLY to
//                            /api/auth/ — nothing else on the host ever sees it.
//                            __Host- cannot be used because it requires Path=/;
//                            __Secure- still forces Secure, and leaving Domain
//                            out keeps it host-only. SameSite=Strict: it is only
//                            ever needed by requests our own pages make.

export const ACCESS_COOKIE = "__Host-ssg-auth";
export const REFRESH_COOKIE = "__Secure-ssg-auth-refresh";
export const REFRESH_PATH = "/api/auth/";

export function accessCookie(token: string, maxAge: number): string {
  return `${ACCESS_COOKIE}=${token}; Max-Age=${maxAge}; Path=/; Secure; HttpOnly; SameSite=Lax`;
}

export function refreshCookie(token: string, maxAge: number): string {
  return `${REFRESH_COOKIE}=${token}; Max-Age=${maxAge}; Path=${REFRESH_PATH}; Secure; HttpOnly; SameSite=Strict`;
}

// clearCookies expires both cookies (same attributes, or the browser keeps them).
export function clearCookies(): string[] {
  return [accessCookie("", 0), refreshCookie("", 0)];
}

// readCookie returns one cookie's value from the request, or "".
export function readCookie(request: Request, name: string): string {
  for (const pair of (request.headers.get("cookie") || "").split(";")) {
    const at = pair.indexOf("=");
    if (at > 0 && pair.slice(0, at).trim() === name) return pair.slice(at + 1).trim();
  }
  return "";
}
