// Banned phrases (#449): the text a reader gets, and how a phrase is found in it.
//
// A phrase is data, never a pattern: it is escaped and compared literally,
// case-insensitively (Unicode simple case folding: "ß" does not match "SS"),
// as a whole word or run of words. `\b` knows only ASCII letters, so the
// edges are Unicode-aware instead: where the phrase starts (ends) with a
// letter, a digit or a combining mark, the text must not have one of those
// right before (after) it. "kup" is then not found in "zakupy", and "कर" not
// in "करें" (the vowel sign is a combining mark). Both sides are normalised
// to NFC, soft hyphens and zero-width spaces are removed, and every run of
// whitespace counts as one space, so a line break inside a phrase still
// matches. A literal pattern with two look-arounds has no nested quantifier:
// matching is linear in the text, and the phrase count and length are bounded
// (content-rules-config.ts).

import type { HtmlNode } from "./sanitize";

// U+00AD soft hyphen, U+200B zero-width space, U+2060 word joiner, U+FEFF.
// Not U+200C/U+200D: they shape Devanagari conjuncts and are part of a word.
const INVISIBLE = /[­​⁠﻿]/g;
// A URL written out in prose: a phrase inside it is not something a reader
// reads as a sentence, and the address is the author's to choose.
const URL_IN_TEXT = /(?:https?:\/\/|mailto:)\S+/gi;
const WORD = /[\p{L}\p{N}\p{M}]/u;
const NOT_BEFORE = String.raw`(?<![\p{L}\p{N}\p{M}])`;
const NOT_AFTER = String.raw`(?![\p{L}\p{N}\p{M}])`;
// Inline elements join their text to the neighbours'; every other element
// (a paragraph, a list item, a cell, a line break) separates words.
const INLINE = new Set(["a", "b", "strong", "em", "i", "u", "s", "del", "sup", "sub", "small", "span", "code"]);

export const normalise = (s: string): string => s.normalize("NFC").replace(INVISIBLE, "").replace(/\s+/g, " ").trim();

// scannable: a text as the phrases are compared with it — normalised, with
// the URLs written in it taken out.
export const scannable = (s: string): string => normalise(s.normalize("NFC").replace(URL_IN_TEXT, " "));

// readerText is the text of a sanitized body as a reader gets it: the words,
// the link texts, and the alt text of each image (what a reader whose mail
// hides images, or a screen reader, gets instead). Never an attribute that
// is an address (href, src) and never a title tooltip.
export function readerText(nodes: HtmlNode[]): string {
  let out = "";
  const walk = (list: HtmlNode[]) => {
    for (const n of list) {
      if (n.type === "text") {
        out += n.text;
      } else if (n.tag === "img") {
        out += ` ${n.attrs.find(([k]) => k === "alt")?.[1] ?? ""} `;
      } else {
        const gap = INLINE.has(n.tag) ? "" : " ";
        out += gap;
        walk(n.children);
        out += gap;
      }
    }
  };
  walk(nodes);
  return out;
}

const escapeRegExp = (s: string): string => s.replace(/[.*+?^${}()|[\]\\]/g, String.raw`\$&`);

// phraseRegExp compiles one normalised phrase.
export function phraseRegExp(phrase: string): RegExp {
  const chars = [...phrase];
  const before = WORD.test(chars[0]) ? NOT_BEFORE : "";
  const after = WORD.test(chars.at(-1) as string) ? NOT_AFTER : "";
  return new RegExp(before + escapeRegExp(phrase) + after, "giu");
}

export interface PhraseHit {
  phrase: string; // as configured (normalised)
  match: string; // the first occurrence as the text has it
  count: number; // how many times it occurs
}

// phraseHits finds each phrase in an already scannable text.
export function phraseHits(text: string, phrases: { phrase: string; re: RegExp }[]): PhraseHit[] {
  const out: PhraseHit[] = [];
  for (const { phrase, re } of phrases) {
    const found = text.match(re);
    if (found) out.push({ phrase, match: found[0], count: found.length });
  }
  return out;
}
