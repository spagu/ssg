// The Modules view's state and actions (GO-149): loading the view, a switch,
// a move, saving labels and a note, previewing and applying an import. No
// DOM: main.js passes `redraw` (draw the page again), `announce` (put a
// sentence in the page's polite live region) and `changed` (tell the shared
// navigation to ask again). Every action awaits its request and catches its
// failure into the state — none is left floating.

import { draftOf, draftPatch, moveOrder, shownTitle, tooLarge } from "./manage.js";

export function createManage({ api, app, redraw, announce, changed = () => undefined }) {
  const state = { view: null, error: null, loading: false, busy: false, failed: null, drafts: {}, open: {}, preview: null, importText: null, importError: null };
  const t = (key, vars) => app.t(key, vars);
  const item = (id) => state.view.items.find((i) => i.id === id);
  const name = (id) => shownTitle(item(id), app.lang);

  async function load() {
    state.loading = true;
    redraw();
    try {
      state.view = await api.get("manage", { lang: app.lang });
      state.error = null;
      state.drafts = Object.fromEntries(state.view.items.map((i) => [i.id, draftOf(i)]));
    } catch (e) {
      state.error = e;
    }
    state.loading = false;
    redraw();
  }

  // run: one change — busy while it lasts, the view reloaded after it, its
  // failure kept for the view to show (failed: {where, error}).
  async function run(where, request, said) {
    state.busy = true;
    state.failed = null;
    redraw();
    try {
      await request();
      changed();
      await load();
      announce(said());
    } catch (e) {
      state.failed = { where, error: e };
      announce(t("manage.failed"));
    }
    state.busy = false;
    redraw();
  }

  const toggle = (id, hidden) =>
    run(id, () => api.send("PATCH", `manage/modules/${encodeURIComponent(id)}`, { hidden }), () => t(hidden ? "manage.hidden.done" : "manage.shown.done", { module: name(id) }));

  async function move(id, dir) {
    const ids = state.view.items.map((i) => i.id);
    const next = moveOrder(ids, id, dir);
    if (!next) {
      announce(t(dir < 0 ? "manage.move.first" : "manage.move.last", { module: name(id) }));
      return;
    }
    await run(id, () => api.send("PUT", "manage/order", { order: next }), () => t("manage.moved", { module: name(id), position: next.indexOf(id) + 1, count: next.length }));
  }

  // edit keeps what is typed; nothing is redrawn, so the field keeps focus.
  function edit(id, field, value, lang = null) {
    const d = state.drafts[id];
    if (field === "label") d.labels[lang] = value;
    else d.note = value;
  }

  // setOpen remembers which "Name and note" boxes are open, so a redraw
  // after a change does not close the one being used.
  function setOpen(id, open) {
    state.open[id] = open;
  }

  async function save(id) {
    const patch = draftPatch(item(id), state.drafts[id]);
    if (!patch) {
      announce(t("manage.save.none"));
      return;
    }
    await run(id, () => api.send("PATCH", `manage/modules/${encodeURIComponent(id)}`, patch), () => t("manage.save.done", { module: name(id) }));
  }

  async function importFile(file) {
    state.preview = null;
    state.importError = null;
    if (!file) return redraw();
    if (tooLarge(file, state.view.limits.importBytes)) {
      state.importError = { kind: "too_large" };
      announce(t("manage.import.tooLarge"));
      return redraw();
    }
    try {
      const text = await file.text();
      state.preview = await api.send("POST", "manage/import", text);
      state.importText = text;
      announce(t("manage.import.previewed", { count: state.preview.changes.length }));
    } catch (e) {
      state.importError = e;
      announce(t("manage.failed"));
    }
    redraw();
  }

  async function applyImport() {
    const { fingerprint, changes } = state.preview;
    await run("import", () => api.send("POST", "manage/import", state.importText, { apply: fingerprint }), () => t("manage.import.applied", { count: changes.length }));
    if (!state.failed) cancelImport();
  }

  function cancelImport() {
    state.preview = null;
    state.importText = null;
    state.importError = null;
    redraw();
  }

  return { state, load, toggle, move, edit, setOpen, save, importFile, applyImport, cancelImport };
}
