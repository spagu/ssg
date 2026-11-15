// GET {base}/api/widgets and the MCP tool widgets_list (GO-174): what an
// author can insert into an article, from three sources in one list —
//   1. the site's typed components (widget-manifest.ts: the published
//      components.json), with their props;
//   2. the shortcodes .ssg.yaml defines (`{{name}}`; `[name]` too when
//      shortcode_brackets is on);
//   3. the built-in Markdown blocks ssg renders — table, code block,
//      footnote, video embed always; Mermaid and math only when the site
//      turns them on (`mermaid: true`, `math: true`).
//
//   → {items: [Widget], nextCursor: null, components: {status, url, hint?,
//      httpStatus?}, features: {mermaid, math, shortcodeBrackets}}
//
// `syntax` is text to insert as it stands (a component's shortest valid call
// from the manifest); the panel builds the full call from the props. A site
// read from MDDB alone has no .ssg.yaml here: no shortcodes, and only the
// blocks that need no switch.

import { contentScope } from "./content-scope";
import { mddbConfigured } from "./mddb-config";
import type { Op } from "./ops";
import { PERM } from "./permissions";
import { repositoryOf } from "./repo";
import type { SiteModel } from "./site-model";
import { readComponents } from "./widget-manifest";
import type { ComponentProp, ComponentsSource } from "./widget-manifest";

export type BlockName = "table" | "code" | "mermaid" | "math" | "footnote" | "embed";
export type Widget =
  | { kind: "component"; name: string; title: string; description: string; syntax: string; props: ComponentProp[] }
  | { kind: "shortcode"; name: string; title: string; description: string; syntax: string }
  | { kind: "block"; name: BlockName; title: string; description: string; syntax: string };
export interface Features { mermaid: boolean; math: boolean; shortcodeBrackets: boolean }
export interface WidgetList {
  items: Widget[];
  nextCursor: null;
  components: Omit<ComponentsSource, "items">;
  features: Features;
}

// BLOCKS: ssg's Markdown (goldmark with tables and footnotes, always on;
// internal/generator buildMarkdown) and its [embed] (YouTube addresses only).
const BLOCKS: { name: BlockName; needs?: keyof Features; title: string; description: string; syntax: string }[] = [
  { name: "table", title: "Table", description: "A table with a header row; columns separated by |.", syntax: "| Column 1 | Column 2 |\n| --- | --- |\n| \u2026 | \u2026 |" },
  { name: "code", title: "Code block", description: "Code in a fenced block; the word after ``` names its language.", syntax: "```go\n\u2026\n```" },
  { name: "mermaid", needs: "mermaid", title: "Mermaid diagram", description: "A diagram written in Mermaid, drawn on the page.", syntax: "```mermaid\ngraph TD\n  A --> B\n```" },
  { name: "math", needs: "math", title: "Formula", description: "Display math between $$ lines, typeset with KaTeX.", syntax: "$$\nE = mc^2\n$$" },
  { name: "footnote", title: "Footnote", description: "A mark in the text and its note at the end of the article.", syntax: "Text[^1]\n\n[^1]: The note." },
  { name: "embed", title: "Video embed", description: "A YouTube video, from its watch or youtu.be address.", syntax: "[embed]https://www.youtube.com/watch?v=VIDEO_ID[/embed]" },
];

export function featuresOf(model: SiteModel | null): Features {
  return { mermaid: model?.mermaid === true, math: model?.math === true, shortcodeBrackets: model?.shortcodeBrackets === true };
}

// blocksFor: the built-in blocks this site renders.
export const blocksFor = (f: Features): Widget[] =>
  BLOCKS.filter((b) => !b.needs || f[b.needs]).map(({ name, title, description, syntax }) => ({ kind: "block", name, title, description, syntax }));

function shortcodesOf(model: SiteModel | null, f: Features): Widget[] {
  const out: Widget[] = (model?.shortcodes ?? []).map((s) => ({
    kind: "shortcode",
    name: s.name,
    title: s.title ?? s.name,
    description: f.shortcodeBrackets ? `{{${s.name}}} or [${s.name}]` : `{{${s.name}}}`,
    syntax: `{{${s.name}}}`,
  }));
  return out.sort((a, b) => a.name.localeCompare(b.name));
}

export const widgetsListOp: Op<void, WidgetList> = {
  name: "widgets_list",
  permission: PERM.read,
  run: async (ctx) => {
    // As site_info: without a repository and without MDDB, contentScope
    // answers the 503 that names what to set.
    const model = repositoryOf(ctx.env, ctx.deps).ok || !mddbConfigured(ctx.env) ? (await contentScope(ctx)).model : null;
    const features = featuresOf(model);
    const { items: defs, ...components } = await readComponents(ctx);
    const comps: Widget[] = defs.map((c) => ({ kind: "component", name: c.name, title: c.name, description: c.description, syntax: c.example, props: c.props }));
    return { items: [...comps, ...shortcodesOf(model, features), ...blocksFor(features)], nextCursor: null, components, features };
  },
};
