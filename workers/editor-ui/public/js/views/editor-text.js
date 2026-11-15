// The Markdown editor itself: a <textarea> (not contenteditable — the text
// is the article, byte for byte) under a bar of buttons. Buttons, shortcuts
// and the Markdown-following rules all go through models/ (pure functions)
// and one history, so Undo takes back whatever any of them did. Pasting or
// dropping an image hands the files to `onImages`. The image button opens
// the media library when the site has one (`onLibrary`), the Insert button
// the widget list (`onInsert`, GO-174).

import { h } from "../core/vnode.js";
import * as cmd from "../models/md-commands.js";
import { continueOnEnter, indent, pasteLink, wrapSelection } from "../models/md-rules.js";
import { createHistory } from "../models/history.js";
import { commandFor, isMac, keysOf } from "../models/shortcuts.js";

// BAR: the buttons in groups; the visible mark of each (the full name is in
// the strings, read by screen readers and shown as the tooltip).
const BAR = [
  [["bold", "B"], ["italic", "I"], ["code", "`"], ["link", "[ ]"]],
  [["h2", "H2"], ["h3", "H3"], ["h4", "H4"]],
  [["bullet", "\u2022"], ["number", "1."], ["task", "\u2610"], ["quote", "\u201c"]],
  [["codeBlock", "{ }"], ["table", "\u25a6"], ["image", "\u25a3"], ["rule", "\u2015"], ["insert", "+"]],
  [["undo", "\u21b6"], ["redo", "\u21b7"], ["help", "?"]],
];
const WRAP_KEYS = new Set(["*", "_", "`", "~", '"', "(", "[", "{"]);

// KEYS: aria-keyshortcuts wants the DOM key names ("Control+B").
const ariaKeys = (command, mac) => keysOf(command, false).replace("Ctrl", mac ? "Meta" : "Control");

export function textEditor(app, { value, onChange, onImages, onHelp, canImage, onInsert, onLibrary, hasLibrary }) {
  const { t } = app;
  const mac = isMac(navigator.userAgentData?.platform ?? navigator.platform);
  const history = createHistory({ text: value, start: 0, end: 0 });
  let area = null;
  // kept: the selection when the field lost focus — a dialog in between (the
  // image's alt text) must not move where the image goes.
  let kept = null;
  const state = () => {
    const sel = document.activeElement !== area && kept ? kept : { start: area.selectionStart, end: area.selectionEnd };
    return { text: area.value, start: Math.min(sel.start, area.value.length), end: Math.min(sel.end, area.value.length) };
  };
  const show = (s) => {
    area.value = s.text;
    area.setSelectionRange(s.start, s.end);
    onChange(s.text);
  };
  const apply = (r) => {
    if (!r) return false;
    show(r);
    history.record(r, Date.now(), false);
    return true;
  };
  const placeholder = (k) => t(`ph.${k}`);
  const COMMANDS = {
    bold: (s) => cmd.toggleInline(s.text, s, "**", placeholder("bold")),
    italic: (s) => cmd.toggleInline(s.text, s, "_", placeholder("italic")),
    code: (s) => cmd.toggleInline(s.text, s, "`", placeholder("code")),
    link: (s) => cmd.link(s.text, s, placeholder("link")),
    h2: (s) => cmd.heading(s.text, s, 2),
    h3: (s) => cmd.heading(s.text, s, 3),
    h4: (s) => cmd.heading(s.text, s, 4),
    bullet: (s) => cmd.linePrefix(s.text, s, "bullet"),
    number: (s) => cmd.linePrefix(s.text, s, "number"),
    task: (s) => cmd.linePrefix(s.text, s, "task"),
    quote: (s) => cmd.linePrefix(s.text, s, "quote"),
    codeBlock: (s) => cmd.codeBlock(s.text, s),
    table: (s) => cmd.table(s.text, s, [placeholder("col1"), placeholder("col2"), placeholder("cell")]),
    rule: (s) => cmd.rule(s.text, s),
    undo: () => history.undo(),
    redo: () => history.redo(),
  };
  // run: one command by name; the image, insert, help and save commands leave the text.
  function run(name) {
    if (name === "image") return hasLibrary() ? onLibrary() : canImage() ? onImages(null) : apply(cmd.image(area.value, state(), placeholder("alt"), "/"));
    if (name === "insert") return onInsert();
    if (name === "help") return onHelp();
    if (name === "save") return onChange(area.value, true);
    const r = COMMANDS[name](state());
    if (name === "undo" || name === "redo") return r ? (show(r), true) : false;
    area.focus();
    return apply(r);
  }
  // escaped: Escape was the last key — the next Tab leaves the field even
  // in a list (the way out of the text field is never blocked).
  let escaped = false;
  function onKey(e) {
    if (e.isComposing) return;
    const wasEscaped = escaped;
    escaped = e.key === "Escape";
    const named = commandFor({ key: e.key, code: e.code, ctrlKey: e.ctrlKey, metaKey: e.metaKey, shiftKey: e.shiftKey, altKey: e.altKey, altGraph: e.getModifierState?.("AltGraph") }, mac);
    if (named) {
      e.preventDefault();
      void run(named);
      return;
    }
    if (e.ctrlKey || e.metaKey || e.altKey) return;
    let r = null;
    if (e.key === "Enter" && !e.shiftKey) r = continueOnEnter(area.value, state());
    else if (e.key === "Tab" && !wasEscaped) r = indent(area.value, state(), e.shiftKey);
    else if (WRAP_KEYS.has(e.key)) r = wrapSelection(area.value, state(), e.key);
    if (r) {
      e.preventDefault();
      apply(r);
    }
  }
  function onPaste(e) {
    const files = [...(e.clipboardData?.files ?? [])].filter((f) => f.type.startsWith("image/"));
    if (files.length) {
      e.preventDefault();
      void onImages(files);
      return;
    }
    const r = pasteLink(area.value, state(), e.clipboardData?.getData("text/plain") ?? "");
    if (r) {
      e.preventDefault();
      apply(r);
    }
  }
  function onDrop(e) {
    const files = [...(e.dataTransfer?.files ?? [])].filter((f) => f.type.startsWith("image/"));
    if (!files.length) return;
    e.preventDefault();
    void onImages(files);
  }
  const button = ([name, mark]) => {
    const keys = keysOf(name, mac);
    const label = t(`tb.${name}`);
    return h("button", { type: "button", class: "tb-btn", title: keys ? `${label} (${keys})` : label, "aria-keyshortcuts": keys ? ariaKeys(name, mac) : null, on: { click: () => void run(name) } },
      h("span", { "aria-hidden": "true" }, mark), h("span", { class: "visually-hidden" }, label));
  };
  const node = h("div", { class: "md-editor" },
    h("div", { class: "md-toolbar", role: "toolbar", "aria-label": t("editor.toolbar"), "aria-controls": "md" }, BAR.map((group) => h("div", { class: "tb-group" }, group.map(button)))),
    h("label", { for: "md", class: "md-label" }, t("editor.text")),
    h("textarea", {
      id: "md", class: "code md-text", rows: "24", spellcheck: "true", "aria-describedby": "md-hint", value,
      ref: (el) => (area = el),
      on: {
        keydown: onKey, paste: onPaste, drop: onDrop,
        blur: () => (kept = { start: area.selectionStart, end: area.selectionEnd }),
        // Back from a dialog the browser may put the caret at the start: put it back.
        focus: () => {
          if (kept) area.setSelectionRange(Math.min(kept.start, area.value.length), Math.min(kept.end, area.value.length));
          kept = null;
        },
        input: () => (history.record(state(), Date.now(), true), onChange(area.value)),
        beforeinput: (e) => {
          if (e.inputType !== "historyUndo" && e.inputType !== "historyRedo") return;
          e.preventDefault();
          void run(e.inputType === "historyUndo" ? "undo" : "redo");
        },
      },
    }),
    h("p", { id: "md-hint", class: "hint" }, t("editor.hint")));
  return {
    node,
    // insert: a change made outside (an image pasted, a revision restored).
    insert: (fn) => apply(fn(area.value, state())),
    focus: () => area.focus(),
    run,
  };
}
