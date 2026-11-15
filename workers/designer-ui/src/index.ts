// designer-ui — the site's designer: every setting of the site (written to
// the .ssg.designer.json overlay, never to .ssg.yaml) and the theme's files,
// in the site's repository, one commit per publication, behind `auth`
// accounts only. A standalone Worker (`wrangler deploy` from this directory).
// This file only wires the entry point to the pipeline (app.ts), with the
// preview's headers around it (preview-entry.ts, GO-188).

import { handleFetch } from "../portable/app";
import type { Env } from "./http";
import { withPreview } from "./preview-entry";

export default {
  fetch(request: Request, env: Env): Promise<Response> {
    return withPreview(request, env, (r) => handleFetch(r, env));
  },
};
