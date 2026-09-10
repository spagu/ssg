# Components

The worked example the documentation refers to. See
[docs/COMPONENTS.md](../../docs/COMPONENTS.md).

It lives here rather than in the default `components/` directory on purpose: a
build run from the repository root would otherwise pick it up implicitly, and
this project's own corpora are built from here. The documentation site points at
it with `components_dir: examples/components`.

A component is a directory: `component.yaml` declares its props, `template.html`
renders them, and anything in `assets/` is copied and linked only on the pages
that use it.
