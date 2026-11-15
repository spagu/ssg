// The Options tab as data: every entry GET api/settings answers, placed in a
// group, with the object keys it sits under as section headings. Pure — the
// view renders it, the tests check that no key of the schema goes missing.
//
// Placement (ticket GO-179):
//   - an entry of the class "plain" stays in its schema group;
//   - an entry of the class "executes" or "secret" goes to the group
//     ADVANCED — visible to everyone who may read, editable only with
//     designer-ui:advanced — under a sub-heading naming its schema group;
//   - an object key that names its fields (mddb, story) is a heading, not a
//     field: its fields are entries of their own. Its heading is repeated in
//     each group its fields land in (mddb is "executes", mddb.lang "plain").
//     An object key written in its short form (`story: true`) or one that
//     accepts keys it does not name is also a field, for that value.

export const ADVANCED = "advanced";

const pathOf = (segments) => segments.join(".");

// isSection: an entry that is only a heading for its fields.
export const isSection = (e) => e.node.type === "object" && !e.node.open && e.value === undefined && e.node.alternate === undefined;

// groupOf: the group an entry is shown in.
export const groupOf = (e) => (e.class === "plain" ? e.node.group ?? "site" : ADVANCED);

// buildForm: {groups: [{id, title, description, items}], entries: Map path → entry}.
// items: [{type: "section" | "field", entry, depth}] in schema order.
export function buildForm(view) {
  const ids = [...view.groups.map((g) => g.id), ADVANCED];
  const items = new Map(ids.map((id) => [id, []]));
  const byPath = new Map(view.entries.map((e) => [pathOf(e.segments), e]));
  const headed = new Map(ids.map((id) => [id, new Set()]));
  for (const e of view.entries) {
    if (isSection(e)) continue;
    // A group the schema does not list (a newer ssg): the first one, never lost.
    const g = items.has(groupOf(e)) ? groupOf(e) : ids[0];
    const list = items.get(g);
    // The headings above this entry, outermost first, each once per group.
    for (let i = 1; i < e.segments.length; i++) {
      const p = pathOf(e.segments.slice(0, i));
      if (headed.get(g).has(p) || !byPath.has(p)) continue;
      headed.get(g).add(p);
      list.push({ type: "section", entry: byPath.get(p), depth: i - 1 });
    }
    list.push({ type: "field", entry: e, depth: e.segments.length - 1 });
  }
  const meta = new Map(view.groups.map((g) => [g.id, g]));
  return {
    groups: ids.map((id) => ({ id, title: meta.get(id)?.title ?? "Advanced", description: meta.get(id)?.description ?? "", items: items.get(id) })),
    entries: byPath,
  };
}

// fieldsOf: the field items of the form, in order, across groups.
export const fieldsOf = (form) => form.groups.flatMap((g) => g.items.filter((i) => i.type === "field").map((i) => i.entry));

// normalise: lower case, accents and punctuation folded, so "pretty-urls",
// "Pretty URLs" and "pretty_urls" all find the same key.
export const normalise = (s) =>
  String(s ?? "")
    .normalize("NFKD")
    .replace(/\p{M}/gu, "")
    .toLowerCase()
    .replace(/[_.\-[\]*]+/g, " ")
    .replace(/\s+/g, " ")
    .trim();

// matches: does the entry answer the query? Every word must appear in the
// key's path, its label (in the panel's language and English) or its
// description (both languages).
export function matches(entry, query, texts) {
  const words = normalise(query).split(" ").filter(Boolean);
  if (!words.length) return true;
  const hay = normalise([pathOf(entry.segments), ...texts].join(" "));
  return words.every((w) => hay.includes(w));
}

// filterForm: the form with only the fields that answer `query` and, with
// `changed`, those set in the panel (the overlay) or edited and not yet
// published. Headings stay above a field that remains; empty groups go.
// `textsOf(entry)` gives the label and description texts to search.
export function filterForm(form, { query = "", changed = false, pending = new Set(), textsOf = (e) => [e.node.description ?? ""] } = {}) {
  const keep = (e) => (!changed || e.source === "overlay" || pending.has(pathOf(e.segments))) && matches(e, query, textsOf(e));
  const groups = [];
  for (const g of form.groups) {
    const fields = g.items.filter((i) => i.type === "field" && keep(i.entry));
    if (!fields.length) continue;
    const under = (sec, f) => f.entry.segments.length > sec.entry.segments.length && pathOf(f.entry.segments).startsWith(pathOf(sec.entry.segments) + ".");
    const items = g.items.filter((i) => (i.type === "field" ? fields.includes(i) : fields.some((f) => under(i, f))));
    groups.push({ ...g, items, count: fields.length });
  }
  return groups;
}

// counts: per group id, how many fields it has and how many come from the panel.
export function counts(form) {
  return Object.fromEntries(form.groups.map((g) => {
    const f = g.items.filter((i) => i.type === "field");
    return [g.id, { fields: f.length, overlay: f.filter((i) => i.entry.source === "overlay").length }];
  }));
}

// humanise: a key as words — "home_posts_limit" → "Home posts limit".
export function humanise(key) {
  const s = String(key ?? "").replace(/[_-]+/g, " ").trim();
  return s ? s[0].toUpperCase() + s.slice(1) : "";
}
