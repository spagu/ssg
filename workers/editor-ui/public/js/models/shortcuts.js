// The editor's keyboard shortcuts: one table for the key handler, the button
// tooltips and the "?" list, so the three never disagree. "Mod" is Ctrl, or
// Command on a Mac. Pure: a key event's fields in, a command id out.

// SHORTCUTS: [command, key, {shift, alt}] — key is KeyboardEvent.key in lower
// case, or KeyboardEvent.code for keys whose character Shift or Alt changes.
export const SHORTCUTS = [
  ["bold", "b"],
  ["italic", "i"],
  ["link", "k"],
  ["code", "e"],
  ["h2", "Digit2", { alt: true }],
  ["h3", "Digit3", { alt: true }],
  ["h4", "Digit4", { alt: true }],
  ["bullet", "Digit8", { shift: true }],
  ["number", "Digit7", { shift: true }],
  ["task", "Digit9", { shift: true }],
  ["quote", "Period", { shift: true }],
  ["codeBlock", "KeyC", { alt: true }],
  ["undo", "z"],
  ["redo", "z", { shift: true }],
  ["redo", "y"],
  ["save", "s"],
];

// isMac: whether the browser runs on a Mac (Command, not Ctrl, is Mod there).
export const isMac = (platform) => /mac|iphone|ipad/i.test(String(platform || ""));

// commandFor: the command a key press means, or null. `e` has key, code,
// ctrlKey, metaKey, shiftKey, altKey and altGraph (getModifierState). AltGr
// arrives as Ctrl+Alt on Windows: it types letters (Polish ć is AltGr+C,
// German ² AltGr+2), so a press with AltGr is never a shortcut.
export function commandFor(e, mac) {
  if (e.altGraph || !(mac ? e.metaKey && !e.ctrlKey : e.ctrlKey && !e.metaKey)) return null;
  const key = String(e.key || "").toLowerCase();
  for (const [command, k, opts = {}] of SHORTCUTS) {
    if (!!opts.shift !== !!e.shiftKey || !!opts.alt !== !!e.altKey) continue;
    if (/^(Digit|Key|Period)/.test(k) ? e.code === k : key === k) return command;
  }
  return null;
}

const KEY_NAME = { Period: ".", KeyC: "C" };
// keysOf: how a shortcut is written for people ("Ctrl+Shift+8", or with the
// Mac's symbols). The first shortcut of a command is the one shown.
export function keysOf(command, mac) {
  const hit = SHORTCUTS.find(([c]) => c === command);
  if (!hit) return "";
  const [, k, opts = {}] = hit;
  const key = KEY_NAME[k] ?? k.replace(/^Digit/, "").toUpperCase();
  if (mac) return `${opts.alt ? "\u2325" : ""}${opts.shift ? "\u21e7" : ""}\u2318${key}`;
  return ["Ctrl", opts.alt ? "Alt" : null, opts.shift ? "Shift" : null, key].filter(Boolean).join("+");
}

// HELP_ONLY: keys the editor understands that are no shortcut of a button.
export const HELP_ONLY = [["enterList", "Enter"], ["tabIndent", "Tab / Shift+Tab"], ["wrap", "* _ ` ~ \" ( [ {"], ["pasteLink", "Ctrl+V"], ["pasteImage", "Ctrl+V"], ["help", "?"]];
