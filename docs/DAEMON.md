# Running several projects at once

`ssg --watch` serves one project. Four sites means four terminals, four
scrollbacks and four things to remember to restart. `ssg daemon` is that,
declared once.

```bash
ssg daemon
```

It reads `.ssg_projects` from the current directory, starts every project in it,
and keeps them running until you stop it.

## The projects file

```yaml
projects:
  - name: blog
    dir: /srv/blog
    port: 8801

  - name: shop
    dir: /srv/shop
    port: 8802

  - name: docs
    dir: ../docs-site          # relative to this file
    config: .ssg.prod.yaml
    args: ["--minify-all"]

  - name: staging
    dir: /srv/staging
    disabled: true             # kept in the file, not running
```

| Key | Meaning |
|---|---|
| `dir` | **Required.** The project root — where its config lives and where its build runs. Relative paths resolve against the projects file, so a checkout can be moved without editing it |
| `name` | Identifies the project in the log and on reload. Defaults to the directory's base name |
| `config` | The project's own config file, relative to `dir`. Empty lets ssg find it the way a plain build does |
| `port` | Serves the project here. **A port implies `http: true`** — asking for one is the clearest statement that a server was meant |
| `http` | Serve without naming a port, letting ssg choose. Only useful for one project: four sites on one machine want four numbers |
| `host` | Bind address for this project's server |
| `args` | Extra flags handed to the build verbatim — `--minify-all`, `--drafts` — so a project is not limited to what this file models |
| `disabled` | Keep a project in the file without running it: the reason to comment one out, without commenting it out |

Two projects may not share a name or a port, and a file that could not run is
refused rather than half-started. A daemon that brings up three of four sites and
says nothing about the fourth is worse than one that does not start.

## Checking the file

```bash
ssg daemon --once
```

Prints what would run and exits, leaving nothing behind — a projects-file check
for CI:

```
   blog → /srv/blog    ssg [--watch --http --port=8801]
   docs → /srv/docs    ssg [--watch --config=.ssg.prod.yaml --minify-all]
   shop → /srv/shop    ssg [--watch --http --port=8802]
   (1 disabled)
```

Each project is an ordinary `ssg --watch` in its own directory. That is the
point: the daemon adds supervision, not a second way to build a site.

## Reloading

Save `.ssg_projects` and the fleet reconciles itself. **A project whose settings
did not change keeps running and keeps its port** — only what actually changed is
touched:

```
♻️  Reloading (.ssg_projects changed)
   ⏹️  docs restarting
   ▶️  docs (/srv/docs)
   ▶️  staging (/srv/staging)
   ✅ 4 project(s) running: [blog docs shop staging]
```

Editing one project's port must not rebuild the other three, and adding a fifth
must not interrupt the four — otherwise "reload" is a restart wearing a different
word.

`SIGHUP` reloads on demand, which is what a deploy script should send after
writing the file:

```bash
kill -HUP "$(pgrep -f 'ssg daemon')"
```

**A projects file that no longer parses leaves everything running.** The projects
on disk are still serving, and stopping them over a typo would be the worse
failure:

```
⚠️  .ssg_projects changed: yaml: line 4: did not find expected key
   Keeping the running projects — fix the file and save to retry.
```

## What the daemon does about failures

- **A project that will not start** is named and skipped; the rest come up. Three
  sites serving is better than none.
- **A project that exits on its own** — a build that gave up on a bad config — is
  restarted. A site must not silently go unserved.
- **Stopping** signals the project's whole process group, so a `watch_runner`
  child releases the port too. A project that ignores the signal is ended after
  a grace period, because the next start needs that port.

## Output

Each project's own output is tagged, so one terminal carrying four builds stays
readable:

```
[blog] 🔄 Loading content...
[shop] ✅ Site generated successfully to output/
[docs] 👀 Watching for changes in content, templates, data, config
```

`--quiet` silences the daemon's own lines; the projects still log.

## Running it as a service

The daemon is a plain foreground process, so any supervisor can own it:

```ini
# /etc/systemd/system/ssg.service
[Unit]
Description=SSG — watch several projects
After=network.target

[Service]
ExecStart=/usr/local/bin/ssg daemon --config /srv/.ssg_projects
ExecReload=/bin/kill -HUP $MAINPID
WorkingDirectory=/srv
Restart=on-failure
User=www-data

[Install]
WantedBy=multi-user.target
```

`systemctl reload ssg` then reconciles the fleet without stopping the projects
that did not change.
