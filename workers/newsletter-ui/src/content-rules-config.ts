// Content rules (#449): what wrangler.toml configures, read and checked.
//
//   NEWSLETTER_BANNED_PHRASES          phrases separated by |
//   NEWSLETTER_BLOCKED_IMAGE_PATTERNS  substrings separated by | an image URL must not contain
//   NEWSLETTER_UTM_STRICT              on: a hand-written utm_* on a tagged host is a finding
//   NEWSLETTER_TRACK_SKIP_HOSTS        not a rule, but its problem is reported here (#464, click-skip.ts)
//
// Every rule is off unless set. Configuration lives in wrangler.toml only —
// the panel is read-only about configuration, so a stolen panel session
// cannot loosen the rules. A configuration this file cannot honour is a
// `problem`: scheduling is refused (503) until it is fixed, never silently
// run with part of the rules.

import { skipHostsConfig } from "./click-skip";
import { normalise } from "./content-rules-text";
import type { Env } from "./lib";
import { isOn } from "./lib";
import { utmOn } from "./utm";

// The bounds keep one check cheap and predictable: at most MAX_PHRASES
// literal phrases of at most MAX_PHRASE_LENGTH characters each are compared
// with a mail, and at most MAX_IMAGE_PATTERNS substrings of at most
// MAX_PATTERN_LENGTH characters with each image URL.
export const MAX_PHRASES = 200;
export const MAX_PHRASE_LENGTH = 100;
export const MAX_IMAGE_PATTERNS = 50;
export const MAX_PATTERN_LENGTH = 200;

export interface ContentRules {
  phrases: string[]; // normalised (content-rules-text.ts), distinct ignoring case
  imagePatterns: string[]; // trimmed, distinct ignoring case
  utmStrict: boolean;
}

// What GET /api/settings and the check say about the rules: counts and
// switches, never the phrase list itself.
export interface RulesSummary {
  active: boolean;
  bannedPhrases: number;
  blockedImagePatterns: number;
  utmStrict: boolean;
  problem: string | null;
}

// list splits a variable on "|", trims, drops empty items, and keeps the
// first spelling of items that differ only in case.
function list(raw: string | undefined, norm: (s: string) => string): string[] {
  const seen = new Map<string, string>();
  for (const item of (raw ?? "").split("|").map(norm)) if (item && !seen.has(item.toLowerCase())) seen.set(item.toLowerCase(), item);
  return [...seen.values()];
}

// utmFlag reads NEWSLETTER_UTM_STRICT: true, false, or null for a value that
// is neither an on nor an off word (a typo must not pass as "off").
export function utmFlag(raw: string | undefined): boolean | null {
  if (isOn(raw)) return true;
  return /^(0|false|no|off)?$/i.test((raw ?? "").trim()) ? false : null;
}

const length = (s: string): number => [...s].length; // characters, not UTF-16 units

function tooMany(name: string, items: string[], max: number, maxLength: number, noun: string): string | null {
  if (items.length > max) return `${name} has ${items.length} ${noun}s, at most ${max} are allowed`;
  const long = items.find((p) => length(p) > maxLength);
  return long === undefined ? null : `${name} has a ${noun} longer than ${maxLength} characters: "${[...long].slice(0, 20).join("")}…"`;
}

// rulesConfig reads the three variables. `problem` lists, joined by "; ",
// every reason the configuration cannot be honoured.
export function rulesConfig(env: Env): { rules: ContentRules; problem: string | null } {
  const phrases = list(env.NEWSLETTER_BANNED_PHRASES, normalise);
  const imagePatterns = list(env.NEWSLETTER_BLOCKED_IMAGE_PATTERNS, (s) => s.trim());
  const strict = utmFlag(env.NEWSLETTER_UTM_STRICT);
  const problems = [
    tooMany("NEWSLETTER_BANNED_PHRASES", phrases, MAX_PHRASES, MAX_PHRASE_LENGTH, "phrase"),
    tooMany("NEWSLETTER_BLOCKED_IMAGE_PATTERNS", imagePatterns, MAX_IMAGE_PATTERNS, MAX_PATTERN_LENGTH, "pattern"),
    strict === null ? `NEWSLETTER_UTM_STRICT must be on or off, not "${(env.NEWSLETTER_UTM_STRICT ?? "").trim().slice(0, 20)}"` : null,
    // Strict mode checks links against the worker's own tags; with tagging
    // off there are none, so the setting would promise a check that cannot
    // run. Refusing is cheaper than a send the admin thought was checked.
    strict && !utmOn(env) ? "NEWSLETTER_UTM_STRICT is on but NEWSLETTER_UTM is off: turn NEWSLETTER_UTM on, or remove NEWSLETTER_UTM_STRICT" : null,
    // #464: a mistyped skip host would send a link the admin wanted left
    // alone through the counting redirect; refuse like any other rule.
    skipHostsConfig(env).problem,
  ].filter((p): p is string => p !== null);
  return { rules: { phrases, imagePatterns, utmStrict: strict === true && utmOn(env) }, problem: problems.length ? problems.join("; ") : null };
}

export function summaryOf(rules: ContentRules, problem: string | null): RulesSummary {
  return {
    active: rules.phrases.length > 0 || rules.imagePatterns.length > 0 || rules.utmStrict || problem !== null,
    bannedPhrases: rules.phrases.length,
    blockedImagePatterns: rules.imagePatterns.length,
    utmStrict: rules.utmStrict,
    problem,
  };
}

// rulesSummary: what GET /api/settings says about the content rules.
export function rulesSummary(env: Env): RulesSummary {
  const { rules, problem } = rulesConfig(env);
  return summaryOf(rules, problem);
}
