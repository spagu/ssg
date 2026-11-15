// A secret in the settings is never a value, only a reference to an
// environment variable the build reads (README of the round, decision 8). The
// worker answers a value inside a secret key as {set, reference} and accepts
// only these strict forms (src/redact.ts, STRICT):
//
//   whole   "$NAME"                     jwt_secret, mddb.api_key, …
//   login   "<login>:$NAME"             server_users
//   inline  "[<Scheme> ]$NAME"          a header: "Bearer $TOKEN"
//
// The panel asks for the parts — a variable name, a login, a scheme word —
// and builds the reference, so a pasted secret can never be sent.

export const NAME = /^[A-Za-z_][A-Za-z0-9_]*$/;
const LOGIN = /^[^:\s]{1,64}$/;
const SCHEME = /^[A-Za-z][A-Za-z0-9-]{0,30}$/;

// isRedacted: the worker's {set, reference} stand-in for a secret value.
export const isRedacted = (v) =>
  typeof v === "object" && v !== null && !Array.isArray(v) && typeof v.set === "boolean" && (v.reference === null || typeof v.reference === "string") && Object.keys(v).length === 2;

// parseReference splits a reference in `format` into its parts, or null.
export function parseReference(format, s) {
  if (typeof s !== "string") return null;
  const name = (ref) => {
    const m = /^\$(?:\{([A-Za-z_][A-Za-z0-9_]*)\}|([A-Za-z_][A-Za-z0-9_]*))$/.exec(ref);
    return m ? m[1] ?? m[2] : null;
  };
  if (format === "login") {
    const i = s.lastIndexOf(":");
    const n = i > 0 ? name(s.slice(i + 1)) : null;
    return n && LOGIN.test(s.slice(0, i)) ? { login: s.slice(0, i), scheme: "", name: n } : null;
  }
  if (format === "inline") {
    const m = /^(?:([A-Za-z][A-Za-z0-9-]{0,30}) )?(\S+)$/.exec(s);
    const n = m ? name(m[2]) : null;
    return n ? { login: "", scheme: m[1] ?? "", name: n } : null;
  }
  const n = name(s);
  return n ? { login: "", scheme: "", name: n } : null;
}

// buildReference: the reference for the parts, or {error} naming what is
// wrong: "secretName" (not a variable name), "secretLogin", "secretScheme".
export function buildReference(format, { name = "", login = "", scheme = "" }) {
  const n = name.trim().replace(/^\$/, "");
  if (!NAME.test(n)) return { error: "secretName" };
  if (format === "login") return LOGIN.test(login.trim()) ? { value: `${login.trim()}:$${n}` } : { error: "secretLogin" };
  if (format === "inline" && scheme.trim()) return SCHEME.test(scheme.trim()) ? { value: `${scheme.trim()} $${n}` } : { error: "secretScheme" };
  return { value: `$${n}` };
}

// secretDraft: the parts for the form, from what the worker answered (a
// {set, reference}, a plain reference string, or nothing). `keep` marks a
// secret that is set but not as a reference the panel can show (a literal
// written by hand into .ssg.yaml): it can only be replaced, never kept,
// when the list or map holding it is published again.
export function secretDraft(format, v) {
  const ref = isRedacted(v) ? v.reference : typeof v === "string" ? v : null;
  const set = isRedacted(v) ? v.set : typeof v === "string" && v !== "";
  const parts = parseReference(format, ref) ?? { login: "", scheme: "", name: "" };
  return { kind: "secret", format, ...parts, set, keep: set && !parseReference(format, ref) };
}
