---
title: Cross-compiling Go and Rust for small devices
slug: cross-compiling
status: publish
type: post
date: 2026-02-02
taxonomies:
  technology: [Go, Rust]
  platform: [Linux]
difficulty: Advanced
series: Shipping to embedded targets
---

One binary, five architectures: GOOS/GOARCH matrices on the Go side and
cargo target triples on the Rust side, plus the linker flags that keep the
output small enough for a router image.
