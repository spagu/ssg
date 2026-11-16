# Running and deploying workers

How the worker templates run beside the preview server, where their
`wrangler.toml` comes from, how a site with workers is deployed, and which
Cloudflare set-ups silently keep a Function from running. The keys that wire
them into the build are in [WORKERS-CONFIGURATION.md](WORKERS-CONFIGURATION.md);
the deployment of the site itself is in [DEPLOYMENT.md](DEPLOYMENT.md).

## Local development

```sh
ssg --config .ssg.yaml --http --watch
```

When a functions-mode worker is set, `--watch` serves the pages **and** the
Functions together by running `wrangler pages dev .` from the build output
directory (that is where SSG copies each worker's `functions/`, and where
`wrangler pages dev` looks for them). SSG also generates a starter
`wrangler.toml` first if the project has none (see below), so bindings are
available. A prebuilt `mode: worker` keeps `wrangler dev` from its own
directory. An explicit `watch_runner` (or `--wrangler`/`--workerd`) overrides
all of this.

The runner needs Node.js (`npx wrangler`). If the runner is not installed, the
preview still starts, and ssg says once that `/api/*` is not served. This is
the case in the ssg Docker image, which has no Node. Two ways to handle it:

- Run `npx wrangler pages dev output` on the host, beside the container,
  for the Functions.
- Set `watch_runner: none` to preview the static site alone without the note.

## Generating a wrangler config

`wrangler pages dev` and `wrangler pages deploy` read a `wrangler.toml` for the
build output directory and any bindings. When a project uses workers and has no
wrangler config, SSG writes a starter one — on `--watch`, or on demand:

```sh
ssg new wrangler
```

It derives `name` from the domain and `pages_build_output_dir` from the output
dir, and appends each worker's `wrangler.snippet.toml` — a fragment the worker
ships declaring its bindings and vars (e.g. cookie-consent's optional
`CONSENT_LOG` KV namespace). An existing wrangler config, or one named via a
worker's `wrangler_config`, is never overwritten.

## Deploy

```sh
ssg --config .ssg.yaml --deploy cloudflare --deploy-project my-site
```

- With a `functions/` tree, SSG shells out to `npx wrangler pages deploy` so
  Cloudflare builds the Functions. `CLOUDFLARE_API_TOKEN` and
  `CLOUDFLARE_ACCOUNT_ID` are read from the environment.
- With `mode: worker` (a prebuilt `_worker.js`), the pure-Go Direct Upload path
  is used — no wrangler, no Node.

If wrangler is required but missing, the deploy fails with an actionable
message; switch to `mode: worker` with a prebuilt bundle to avoid the Node
dependency.

## Deployment topologies (which requests reach your Functions)

Whether a `workers:` endpoint actually runs depends on the shape of your
Cloudflare deployment, not just on `routes_include`. Two caveats bite silently.

| Topology | What runs | Functions active? |
|---|---|---|
| Pure static (no `worker:`/`workers:`) | assets only | — |
| Functions mode (`mode: functions`, the default) | `functions/` tree per `_routes.json` | **yes** |
| Advanced mode (a `_worker.js` at the output root) | that single Worker for **every** request | **no** |
| Zone Worker route (e.g. `example.com/api/*`) owns a prefix | the zone Worker on that prefix | shadowed there |

- **Advanced mode disables Functions entirely.** If a `_worker.js` sits at the
  output root (Pages "advanced mode"), Cloudflare runs *only* that Worker and the
  `functions/` tree is ignored — so every `workers:` endpoint is silently dead.
  Pick one: the `functions/` model (`mode: functions`) **or** a single hand-built
  `_worker.js` (`mode: worker`) that does its own routing. SSG's `workers:` merge
  targets the Functions model.
- **Zone-level Worker routes shadow Function routes.** A route like
  `example.com/api/*` bound to a standalone Worker on the zone runs *before* Pages
  Functions, so a `workers:` endpoint under the same prefix (say
  `/api/consent/*`) never fires. Either move the endpoint off the owned prefix, or
  carve it out of the zone route. A quick smoke test after deploy — `curl` each
  endpoint path — catches both cases before your visitors do.

## What SSG does not do

- No JS/TS bundler — Cloudflare Pages builds Functions from source; `mode:
  worker` covers prebuilt bundles.
- No secret management — that is `wrangler pages secret put`.
- No KV/Durable Object provisioning — bind those in the Pages dashboard.
