// #435: the external issuer's variables. Nothing set → absent; anything set
// and not usable → misconfigured with a hint, never a half-open method —
// above all HS* next to a public key (algorithm confusion, RFC 8725).
import { test } from "node:test";
import assert from "node:assert/strict";
import { ASYMMETRIC, externalLookup } from "../functions/api/auth/_external-config.ts";
import type { ExternalSetup } from "../functions/api/auth/_external-config.ts";
import { staticKeys } from "../functions/api/auth/_external-keys.ts";
import type { Env } from "../functions/api/auth/_lib.ts";
import { providerName } from "../functions/api/auth/_provider.ts";
import { methodEnabled, signInMethods } from "../functions/api/auth/_provider-methods.ts";
import { AUD, BASE_ENV, ISS, JWKS, RS, SECRET, testKey } from "./external-fake.ts";

const look = (v: Record<string, string>) => externalLookup(v as unknown as Env);
const setup = (v: Record<string, string>) => {
  const l = look(v);
  assert.equal(l.kind, "enabled", l.kind === "misconfigured" ? l.problem : "");
  return (l as { setup: ExternalSetup }).setup;
};
const problem = (v: Record<string, string>) => {
  const l = look(v);
  assert.equal(l.kind, "misconfigured");
  return (l as { problem: string }).problem;
};

test("off by default: no AUTH_JWT_* variable → absent, no sign-in method", () => {
  assert.deepEqual(look({}), { kind: "absent" });
  assert.equal(signInMethods({} as Env).some((m) => m.id.startsWith("jwt:")), false);
});

test("the defaults: every asymmetric algorithm, 5 minutes, sub, verified addresses, reject unknown people", () => {
  const s = setup(BASE_ENV);
  assert.deepEqual(s, {
    method: `jwt:${ISS}`,
    issuer: ISS,
    audience: AUD,
    algorithms: [...ASYMMETRIC],
    source: { kind: "jwks", url: JWKS, host: "id.example.com" },
    maxAgeSeconds: 300,
    identityClaim: "sub",
    emailVerified: true,
    unknown: "reject",
    rolesClaim: "roles",
    roleMap: null,
    allowAdminRoles: false,
    formOrigins: [],
    linkUrl: "",
  });
  assert.ok(!s.algorithms.includes("none") && !s.algorithms.some((a) => a.startsWith("HS")));
});

test("every option read, trimmed and lower-cased where it is a word", () => {
  const s = setup({
    ...BASE_ENV,
    AUTH_JWT_ALGORITHMS: " ES256 , EdDSA ",
    AUTH_JWT_MAX_AGE_SECONDS: "60",
    AUTH_JWT_IDENTITY_CLAIM: "EMAIL",
    AUTH_JWT_EMAIL_VERIFIED: "off",
    AUTH_JWT_UNKNOWN: "Create",
    AUTH_JWT_ROLES_CLAIM: "https://example.com/roles",
    AUTH_JWT_ROLE_MAP: JSON.stringify({ editor: { roles: ["newsletter-editor"], groups: ["Editors"] }, viewer: {} }),
    AUTH_JWT_ALLOW_ADMIN_ROLES: " On ",
    AUTH_JWT_FORM_ORIGINS: "https://id.example.com/, https://login.example.com",
    AUTH_JWT_LINK_URL: " https://id.example.com/connect?to=example ",
  });
  assert.equal(s.linkUrl, "https://id.example.com/connect?to=example");
  assert.equal(s.allowAdminRoles, true);
  assert.deepEqual(s.algorithms, ["ES256", "EdDSA"]);
  assert.deepEqual([s.maxAgeSeconds, s.identityClaim, s.emailVerified, s.unknown, s.rolesClaim], [60, "email", false, "create", "https://example.com/roles"]);
  assert.deepEqual([...(s.roleMap as Map<string, unknown>)], [["editor", { roles: ["newsletter-editor"], groups: ["Editors"] }], ["viewer", { roles: [], groups: [] }]]);
  assert.deepEqual(s.formOrigins, ["https://id.example.com", "https://login.example.com"]);
});

test("the key source: exactly one; a JWKS over https without credentials; a usable static key; a long enough secret", async () => {
  const issuer = { AUTH_JWT_ISSUER: ISS, AUTH_JWT_AUDIENCE: AUD };
  assert.match(problem(issuer), /exactly one key source/);
  assert.match(problem({ ...BASE_ENV, AUTH_JWT_PUBLIC_KEY: RS.spkiPem }), /exactly one key source/);
  assert.match(problem({ ...BASE_ENV, AUTH_JWT_SECRET: SECRET }), /exactly one key source/);
  assert.match(problem({ ...issuer, AUTH_JWT_JWKS_URL: "not a url" }), /not a URL/);
  assert.match(problem({ ...issuer, AUTH_JWT_JWKS_URL: "http://id.example.com/jwks" }), /https:/);
  assert.match(problem({ ...issuer, AUTH_JWT_JWKS_URL: "https://u:p@id.example.com/jwks" }), /without credentials/);
  assert.match(problem({ ...issuer, AUTH_JWT_PUBLIC_KEY: "not a key" }), /JWK, a JWK Set or a PEM/);
  assert.equal(setup({ ...issuer, AUTH_JWT_PUBLIC_KEY: RS.spkiPem }).source.kind, "static");
  assert.match(problem({ ...issuer, AUTH_JWT_SECRET: "short", AUTH_JWT_ALGORITHMS: "HS256" }), /at least 32 bytes/);
  const hs = setup({ ...issuer, AUTH_JWT_SECRET: SECRET, AUTH_JWT_ALGORITHMS: "HS256,HS512" });
  assert.deepEqual([hs.source.kind, hs.algorithms], ["secret", ["HS256", "HS512"]]);
  const ed = await testKey("EdDSA");
  assert.equal(staticKeys(JSON.stringify({ keys: [ed.jwk, RS.jwk, null, 5] })).length, 2, "a JWK Set: its objects");
  assert.equal(staticKeys(JSON.stringify(ed.jwk)).length, 1, "one JWK");
  assert.equal(staticKeys(RS.spkiPem.replace(/\n/g, "\\n")).length, 1, "a PEM with escaped newlines, as a variable may hold it");
  assert.deepEqual(staticKeys("-----BEGIN PUBLIC KEY-----\n@@@\n-----END PUBLIC KEY-----"), [], "not base64");
  assert.deepEqual(staticKeys("-----BEGIN PUBLIC KEY-----\nQQ=Q\n-----END PUBLIC KEY-----"), [], "bad padding");
});

test("algorithm confusion is a configuration error: HS* with a public key, or a secret with public-key algorithms", () => {
  assert.match(problem({ ...BASE_ENV, AUTH_JWT_ALGORITHMS: "RS256,HS256" }), /need the AUTH_JWT_SECRET secret and no public key/);
  assert.match(problem({ AUTH_JWT_ISSUER: ISS, AUTH_JWT_AUDIENCE: AUD, AUTH_JWT_PUBLIC_KEY: RS.spkiPem, AUTH_JWT_ALGORITHMS: "HS256" }), /algorithm-confusion/);
  assert.match(problem({ AUTH_JWT_ISSUER: ISS, AUTH_JWT_AUDIENCE: AUD, AUTH_JWT_SECRET: SECRET }), /goes only with HS256/, "a secret with the asymmetric default");
  assert.match(problem({ AUTH_JWT_ISSUER: ISS, AUTH_JWT_AUDIENCE: AUD, AUTH_JWT_SECRET: SECRET, AUTH_JWT_ALGORITHMS: "HS256,ES256" }), /goes only with HS256/);
  assert.match(problem({ ...BASE_ENV, AUTH_JWT_ALGORITHMS: "none" }), /does not accept/);
  assert.match(problem({ ...BASE_ENV, AUTH_JWT_ALGORITHMS: "RS256,rs256" }), /rs256/);
});

test("each other variable refused when it is not usable", () => {
  assert.match(problem({ AUTH_JWT_JWKS_URL: JWKS }), /AUTH_JWT_ISSUER/);
  assert.match(problem({ ...BASE_ENV, AUTH_JWT_ISSUER: "has space" }), /AUTH_JWT_ISSUER/);
  assert.match(problem({ ...BASE_ENV, AUTH_JWT_AUDIENCE: "" }), /AUTH_JWT_AUDIENCE/);
  for (const v of ["0", "3601", "5m", "-1"]) assert.match(problem({ ...BASE_ENV, AUTH_JWT_MAX_AGE_SECONDS: v }), /MAX_AGE/, v);
  assert.match(problem({ ...BASE_ENV, AUTH_JWT_IDENTITY_CLAIM: "name" }), /IDENTITY_CLAIM/);
  assert.match(problem({ ...BASE_ENV, AUTH_JWT_EMAIL_VERIFIED: "maybe" }), /EMAIL_VERIFIED/);
  assert.match(problem({ ...BASE_ENV, AUTH_JWT_UNKNOWN: "allow" }), /AUTH_JWT_UNKNOWN/);
  for (const bad of ["not a url", "http://id.example.com/connect", "https://u:p@id.example.com/connect"]) assert.match(problem({ ...BASE_ENV, AUTH_JWT_LINK_URL: bad }), /AUTH_JWT_LINK_URL must be an https:\/\/ URL/);
  assert.match(problem({ ...BASE_ENV, AUTH_JWT_ALLOW_ADMIN_ROLES: "yes" }), /AUTH_JWT_ALLOW_ADMIN_ROLES must be "off" or "on"/);
  assert.match(problem({ ...BASE_ENV, AUTH_JWT_ROLES_CLAIM: "a b" }), /ROLES_CLAIM/);
  for (const m of ["{", "[]", '{"a": []}', '{"a": {"roles": "x"}}', '{"a": {"roles": [""]}}', '{"a": {"perms": []}}', "null"]) {
    assert.match(problem({ ...BASE_ENV, AUTH_JWT_ROLE_MAP: m }), /AUTH_JWT_ROLE_MAP must be/, m);
  }
  assert.match(problem({ ...BASE_ENV, AUTH_JWT_ROLE_MAP: '{"boss": {"roles": ["owner"]}}' }), /owner role is never given/);
  for (const o of ["http://id.example.com", "https://id.example.com/login", "nope"]) assert.match(problem({ ...BASE_ENV, AUTH_JWT_FORM_ORIGINS: o }), /FORM_ORIGINS/, o);
});

test("a sign-in method of its own: jwt:<issuer>, named by its issuer; a broken one offered nowhere and logged", () => {
  const env = BASE_ENV as unknown as Env;
  assert.ok(methodEnabled(env, `jwt:${ISS}`));
  assert.equal(providerName(`jwt:${ISS}`), ISS);
  assert.equal(providerName("password"), "password");
  const quiet = console.error;
  const logged: string[] = [];
  console.error = (m: string) => logged.push(m);
  try {
    assert.equal(signInMethods({ ...BASE_ENV, AUTH_JWT_ALGORITHMS: "HS256" } as unknown as Env).some((m) => m.id.startsWith("jwt:")), false);
    assert.match(logged.join("\n"), /external JWT sign-in is not offered/);
  } finally {
    console.error = quiet;
  }
  // Only the external issuer on: still a way in, so the site is usable.
  assert.deepEqual(signInMethods({ ...BASE_ENV, AUTH_PASSWORD_LOGIN: "off" } as unknown as Env).map((m) => m.id), [`jwt:${ISS}`]);
});
