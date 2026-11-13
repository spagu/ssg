// HTML character references, both ways, for the sanitizer and the renderer.
//
// Decoding matters for safety, not looks: "&#106;avascript:" and
// "java&Tab;script:" are what a browser reads as "javascript:", so a URL is
// checked only after it is decoded, and what is written back out is the decoded
// value, escaped again. The checked string and the emitted string are the same.

// The named references a newsletter plausibly contains, plus the ones used to
// hide a scheme ("&colon;", "&Tab;", "&NewLine;"). An unknown name stays as it
// was written ("&bogus;"), which the escaper then shows literally.
const NAMED: Record<string, string> = {
  amp: "&", lt: "<", gt: ">", quot: '"', apos: "'", nbsp: " ",
  colon: ":", tab: "\t", newline: "\n", sol: "/", lpar: "(", rpar: ")", period: ".",
  copy: "©", reg: "®", trade: "™", hellip: "…", mdash: "—", ndash: "–",
  lsquo: "‘", rsquo: "’", ldquo: "“", rdquo: "”", laquo: "«", raquo: "»", bdquo: "„",
  euro: "€", pound: "£", yen: "¥", cent: "¢", bull: "•", middot: "·", deg: "°",
  times: "×", divide: "÷", para: "¶", sect: "§", shy: "­", zwj: "‍", zwnj: "‌",
};

// fromCodePoint, but a NUL, a surrogate or anything past U+10FFFF becomes
// U+FFFD, as browsers do.
function codePoint(n: number): string {
  if (n === 0 || n > 0x10ffff || (n >= 0xd800 && n <= 0xdfff)) return "�";
  return String.fromCodePoint(n);
}

// decodeEntities decodes numeric references with or without the trailing ";"
// (browsers accept both) and the named ones above, case-insensitively.
export function decodeEntities(s: string): string {
  return s.replace(/&(?:#[xX]([0-9a-fA-F]{1,8})|#([0-9]{1,10})|([A-Za-z][A-Za-z0-9]{0,31}));?/g, (whole, hex, dec, name) => {
    if (hex !== undefined) return codePoint(Number.parseInt(hex, 16));
    if (dec !== undefined) return codePoint(Number.parseInt(dec, 10));
    const v = NAMED[(name as string).toLowerCase()];
    return v ?? whole;
  });
}

// escapeHtml makes any string safe as text content and as a double- or
// single-quoted attribute value.
export function escapeHtml(s: string): string {
  return s.replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[c] as string);
}
