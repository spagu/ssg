// The frame every campaign's HTML part is sent in.
//
// Mail clients are not browsers: many drop <style>, some drop <head>, several
// rewrite colours in dark mode. So the layout leans on what survives all of
// them — a presentation table, inline styles on the frame, system fonts, no
// remote resources (no web fonts, no images of its own) — and adds a small
// <style> block only as an improvement where it is honoured (link colour, a
// dark scheme for clients that announce one).
//
// Colours (Google palette), contrast checked against WCAG 2.2 AA:
//   light  text #202124 on #ffffff 16.1:1 · footer #5f6368 6.0:1 · links #1967d2 5.6:1
//   dark   text #e8eaed on #202124 13.2:1 · footer #bdc1c6  9.0:1 · links #8ab4f8 7.7:1

import { escapeHtml } from "./render-entities";

export interface LayoutInput {
  lang: string; // the <html lang>, the recipient's language
  title: string; // the subject
  content: string; // sanitized HTML
  footer: string; // the footer's HTML (unsubscribe link)
}

const STYLE = [
  ":root{color-scheme:light dark;supported-color-schemes:light dark}",
  "a{color:#1967d2}",
  "@media (prefers-color-scheme:dark){",
  ".nl-bg{background:#202124!important}",
  ".nl-text{color:#e8eaed!important}",
  ".nl-foot{color:#bdc1c6!important;border-color:#5f6368!important}",
  ".nl-bg a{color:#8ab4f8!important}",
  "}",
  "img{max-width:100%;height:auto}",
].join("");

const FONT = "-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,'Noto Sans','Noto Sans Devanagari',Arial,sans-serif";

export function layout(p: LayoutInput): string {
  return (
    `<!doctype html>\n<html lang="${escapeHtml(p.lang)}" dir="ltr">\n<head>\n` +
    `<meta charset="utf-8">\n<meta name="viewport" content="width=device-width, initial-scale=1">\n` +
    `<meta name="color-scheme" content="light dark">\n<meta name="supported-color-schemes" content="light dark">\n` +
    `<title>${escapeHtml(p.title)}</title>\n<style>${STYLE}</style>\n</head>\n` +
    `<body class="nl-bg" style="margin:0;padding:0;background:#ffffff;">\n` +
    `<table role="presentation" class="nl-bg" width="100%" cellpadding="0" cellspacing="0" border="0" style="background:#ffffff;">` +
    `<tr><td align="center" style="padding:24px 16px;">\n` +
    `<table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="max-width:600px;">\n` +
    `<tr><td class="nl-text" style="font-family:${FONT};font-size:16px;line-height:1.6;color:#202124;text-align:left;">\n` +
    `${p.content}\n</td></tr>\n` +
    `<tr><td class="nl-foot" style="padding-top:16px;margin-top:24px;border-top:1px solid #dadce0;font-family:${FONT};` +
    `font-size:13px;line-height:1.5;color:#5f6368;text-align:left;">\n${p.footer}\n</td></tr>\n` +
    `</table>\n</td></tr></table>\n</body>\n</html>\n`
  );
}
