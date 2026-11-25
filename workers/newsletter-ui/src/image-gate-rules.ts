// NEWSLETTER_IMAGES_REQUIRE (#448): which findings of the pre-send image check
// (src/image-check.ts) refuse scheduling. Unset or empty, the check only warns,
// exactly as in 1.9.15. Each rule names the problem codes it enforces:
//
//   alt    no_alt                   missing, empty or whitespace-only alt text
//   store  not_stored               the image is not under the store's base URL
//   load   unreachable, not_probed  the probe failed, or the image was beyond
//                                   the probe bound and so was never shown to load
//
// Fail closed: a value that cannot be applied — an unknown word, or `store`
// with the store off or without a usable address — is a configuration problem
// (GET /api/settings, the module summary) and refuses every schedule with 503.
// A gate that silently does not gate is worse than none: the admin believes
// the rule holds.

import { assetsConfigProblem, assetsEnabled } from "./asset-config";
import type { ImageItem, ImageProblem } from "./image-check";
import type { Env } from "./lib";

export const IMAGE_RULES = ["alt", "store", "load"] as const;
export type ImageRule = (typeof IMAGE_RULES)[number];

// GateProblem: the check's own codes, plus not_probed — an image the check did
// not probe (over MAX_PROBED_IMAGES), which `load` cannot let pass unseen.
export type GateProblem = ImageProblem | "not_probed";

export const ENFORCES: Record<ImageRule, readonly GateProblem[]> = {
  alt: ["no_alt"],
  store: ["not_stored"],
  load: ["unreachable", "not_probed"],
};

export interface ImageRules {
  rules: ImageRule[]; // the enforced rules, in IMAGE_RULES order
  problem: string | null; // why they cannot be applied; scheduling is refused while set
}

const isRule = (word: string): word is ImageRule => (IMAGE_RULES as readonly string[]).includes(word);

// imageRules reads NEWSLETTER_IMAGES_REQUIRE: words separated by commas and/or
// whitespace, any case, repeats ignored.
export function imageRules(env: Env): ImageRules {
  const words = (env.NEWSLETTER_IMAGES_REQUIRE ?? "").toLowerCase().split(/[\s,]+/).filter(Boolean);
  const unknown = [...new Set(words.filter((w) => !isRule(w)))];
  const rules = IMAGE_RULES.filter((r) => words.includes(r));
  return { rules, problem: rulesProblem(env, rules, unknown) };
}

function rulesProblem(env: Env, rules: ImageRule[], unknown: string[]): string | null {
  if (unknown.length) {
    return `NEWSLETTER_IMAGES_REQUIRE: unknown rule ${unknown.map((w) => `"${w.slice(0, 40)}"`).join(", ")}; use alt, store or load, separated by commas`;
  }
  if (!rules.includes("store")) return null;
  // `store` can only be judged against a usable base; without one every image
  // would fail for a fault that is the configuration's, not the author's.
  if (!assetsEnabled(env)) return "NEWSLETTER_IMAGES_REQUIRE includes store, but the image store is off (no NEWSLETTER_ASSETS binding)";
  const assets = assetsConfigProblem(env);
  return assets ? `NEWSLETTER_IMAGES_REQUIRE includes store, but the image store has no usable address: ${assets}` : null;
}

// blockingOf: the problems of one image that the enforced rules turn into a
// refusal. An image without a probe counts as not_probed only under `load`.
export function blockingOf(item: ImageItem, rules: readonly ImageRule[]): GateProblem[] {
  const enforced = new Set(rules.flatMap((r) => ENFORCES[r]));
  const all: GateProblem[] = item.probe === null ? [...item.problems, "not_probed"] : item.problems;
  return all.filter((p) => enforced.has(p));
}

// withRules adds what the panel needs to the check's answer: the enforced
// rules, why they cannot apply, and each image's blocking problems. With the
// switch unset the answer is returned untouched, byte for byte as in 1.9.15.
export function withRules<R extends { items: ImageItem[] }>(report: R, gate: ImageRules) {
  if (!gate.rules.length && !gate.problem) return report;
  const items = report.items.map((i) => ({ ...i, blocking: gate.problem ? [] : blockingOf(i, gate.rules) }));
  return { ...report, items, rules: gate.rules, rulesProblem: gate.problem };
}
