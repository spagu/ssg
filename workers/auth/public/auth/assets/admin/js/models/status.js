// The Status view's judgements, as pure functions over GET
// /api/auth/admin/status — what is fine, what wants attention.

// keyAdvice: "rotate" past the rotation advice, "ok" within it, "unknown"
// when the age was not recorded (a previous key, or none signed yet).
export function keyAdvice(key) {
  if (key.ageDays === null || key.ageDays === undefined) return "unknown";
  return key.rotate ? "rotate" : "ok";
}

// staleness splits the longest wait for a permission change into minutes and seconds.
export const staleness = (seconds) => ({ minutes: Math.floor(seconds / 60), seconds: seconds % 60 });

// warnings: what the Status view lists at its top, most serious first.
export function warnings(s) {
  const out = [];
  if (s.keys.some((k) => k.current && k.rotate)) out.push("keyOld");
  if (s.mfa.state === "misconfigured") out.push("mfaMisconfigured");
  if (s.mfa.policy === "invalid") out.push("mfaPolicyInvalid");
  if (s.mfa.state !== "off" && s.mfa.accountsMissingFactor > 0) out.push("missingFactor");
  if (s.mail.state === "broken") out.push("mailBroken");
  if (s.access === "misconfigured") out.push("accessMisconfigured");
  if (s.registryProblems.length) out.push("registry");
  return out;
}
