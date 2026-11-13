// GO-128: the panel's formatting, theme storage, polling and the batch runner.
import { test } from "node:test";
import assert from "node:assert/strict";
import { createFormat } from "../public/js/core/format.js";
import { LANG_KEY, applyTheme, readStored, readTheme, writeStored, writeTheme } from "../public/js/core/theme.js";
import { createPoller, runConfirmations } from "../public/js/core/tasks.js";

test("format: numbers, dates, durations, relative times through Intl", () => {
  const f = createFormat("en-GB", "UTC");
  assert.equal(f.number(1234567), "1,234,567");
  assert.equal(f.number("x"), "0");
  assert.equal(f.percent(0.346), "34.6%");
  assert.equal(f.percent(Number.NaN), "0%");
  assert.equal(f.date("2026-11-13T10:00:00Z"), "13 Nov 2026");
  assert.equal(f.date(null), "—");
  assert.equal(f.dateTime("bad"), "—");
  assert.match(f.dateTime("2026-11-13T10:05:00Z"), /13 Nov 2026.*10:05/);
  assert.equal(f.day("2026-11-03"), "3 Nov");
  assert.equal(f.dayLong("2026-11-03"), "3 November 2026");
  assert.equal(f.month("2026-11-01"), "Nov");
  assert.equal(f.list(["a", "b", "c"]), "a, b and c");
  const now = Date.parse("2026-11-13T12:00:00Z");
  assert.equal(f.relative("2026-11-13T11:59:30Z", now), "30 seconds ago");
  assert.equal(f.relative("2026-11-13T11:30:00Z", now), "30 minutes ago");
  assert.equal(f.relative("2026-11-13T09:00:00Z", now), "3 hours ago");
  assert.equal(f.relative("2026-11-15T12:00:00Z", now), "in 2 days");
  assert.equal(f.relative(null), "—");
  assert.ok(f.relative(new Date().toISOString()));
  assert.equal(f.duration(42), "42 secs");
  assert.equal(f.duration(4980), "1 hr 23 mins");
  assert.equal(f.duration(600), "10 mins");
  assert.equal(f.duration(null), "—");
  assert.equal(f.duration(-5), "0 secs");
  assert.equal(createFormat("pl").number(1234567).replace(/[\u00a0\u202f]/g, " "), "1 234 567");
  assert.equal(createFormat("hi").locale, "hi");
});

test("theme and stored choices: storage that works, is missing, or throws", () => {
  const store = new Map<string, string>();
  const ok = { getItem: (k: string) => store.get(k) ?? null, setItem: (k: string, v: string) => void store.set(k, v), removeItem: (k: string) => void store.delete(k) };
  const broken = { getItem() { throw new Error("blocked"); }, setItem() { throw new Error("blocked"); }, removeItem() { throw new Error("blocked"); } };
  assert.equal(readTheme(ok), "auto");
  assert.ok(writeTheme(ok, "dark"));
  assert.equal(readTheme(ok), "dark");
  assert.ok(writeTheme(ok, "auto"));
  assert.equal(readTheme(ok), "auto");
  store.set("newsletter-admin-theme", "neon");
  assert.equal(readTheme(ok), "auto");
  assert.equal(readTheme(broken), "auto");
  assert.equal(readTheme(null), "auto");
  assert.equal(writeTheme(broken, "dark"), false);
  const attrs: Record<string, string> = {};
  const root = { setAttribute: (k: string, v: string) => (attrs[k] = v), removeAttribute: (k: string) => delete attrs[k] };
  assert.equal(applyTheme(root, "dark", false), "dark");
  assert.equal(attrs["data-theme"], "dark");
  assert.equal(applyTheme(root, "auto", true), "dark");
  assert.equal(applyTheme(root, "auto", false), "light");
  assert.equal(attrs["data-theme"], undefined);
  assert.ok(writeStored(ok, LANG_KEY, "pl"));
  assert.equal(readStored(ok, LANG_KEY), "pl");
  assert.equal(readStored(ok, "other"), null);
  assert.equal(readStored(broken, LANG_KEY), null);
  assert.equal(readStored(null, LANG_KEY), null);
  assert.equal(writeStored(broken, LANG_KEY, "pl"), false);
});

// fakeTimers runs scheduled callbacks only when told to.
function fakeTimers() {
  const queue: { id: number; fn: () => void }[] = [];
  let n = 0;
  return {
    queue,
    setTimeout: (fn: () => void) => (queue.push({ id: ++n, fn }), n),
    clearTimeout: (id: number) => {
      const i = queue.findIndex((q) => q.id === id);
      if (i >= 0) queue.splice(i, 1);
    },
    async run() {
      const q = queue.shift();
      q?.fn();
      await new Promise((r) => setImmediate(r));
    },
  };
}

test("poller: ticks until done, sleeps while hidden, wakes, stops, survives errors", async () => {
  const timers = fakeTimers();
  let visible = true;
  let ticks = 0;
  const errors: unknown[] = [];
  const p = createPoller({
    interval: 1000,
    timers,
    visible: () => visible,
    onError: (e) => errors.push(e),
    tick: async () => {
      ticks++;
      if (ticks === 2) throw new Error("blip");
      return ticks >= 4;
    },
  });
  await p.start();
  assert.equal(ticks, 1);
  p.wake(); // a timer is pending: no extra tick
  assert.equal(ticks, 1);
  await timers.run();
  assert.equal(ticks, 2);
  assert.equal(errors.length, 1, "an error is reported and polling continues");
  visible = false;
  await timers.run();
  assert.equal(ticks, 2, "hidden: no request");
  assert.equal(timers.queue.length, 0, "and nothing scheduled until woken");
  p.wake();
  assert.equal(ticks, 2, "still hidden");
  visible = true;
  p.wake();
  await new Promise((r) => setImmediate(r));
  assert.equal(ticks, 3);
  await timers.run();
  assert.equal(ticks, 4);
  assert.ok(p.stopped, "done stops it");
  assert.equal(timers.queue.length, 0);
  p.wake();
  assert.equal(ticks, 4);

  const q = createPoller({ interval: 1, timers, tick: async () => false });
  await q.start();
  assert.equal(timers.queue.length, 1);
  q.stop();
  assert.equal(timers.queue.length, 0);
  q.stop();
  await q.start();
  assert.equal(timers.queue.length, 0, "stopped stays stopped");
  // overlapping runs: a wake during a tick does nothing
  let release: () => void = () => {};
  const slow = createPoller({ interval: 1, timers: fakeTimers(), tick: () => new Promise<boolean>((r) => (release = () => r(true))) });
  const first = slow.start();
  slow.wake();
  await slow.start();
  release();
  await first;
  assert.ok(slow.stopped);
  const real = createPoller({ interval: 1, tick: async () => true });
  await real.start();
  assert.ok(real.stopped);
});

test("runConfirmations: follows next, adds up, waits on 429, stops on abort or other errors", async () => {
  const pages = [
    { sent: 2, failed: 1, skipped: 0, failures: [{ id: "a", detail: "x", temporary: false }], remaining: 5, next: "k1" },
    { sent: 3, failed: 0, skipped: 1, failures: [], remaining: 0, next: null },
  ];
  const seen: unknown[] = [];
  let calls = 0;
  const api = {
    async post(path: string, body: Record<string, unknown>) {
      seen.push([path, body]);
      calls++;
      if (calls === 2) throw Object.assign(new Error("rate"), { kind: "rate", retryAfter: 7 });
      if (calls === 3) throw Object.assign(new Error("rate"), { kind: "rate", retryAfter: null });
      return pages.shift();
    },
  };
  const waits: number[] = [];
  const progress: { waiting: number }[] = [];
  const total = await runConfirmations(api, { sleep: async (s: number) => void waits.push(s), onProgress: (p: { waiting: number }) => progress.push(p) });
  assert.deepEqual([total.sent, total.failed, total.skipped, total.remaining, total.batches, total.done, total.waits], [5, 1, 1, 0, 2, true, 2]);
  assert.deepEqual(waits, [7, 60]);
  assert.deepEqual(seen[0], ["subscribers/confirmations", {}]);
  assert.deepEqual(seen[3], ["subscribers/confirmations", { after: "k1" }]);
  assert.equal(progress.filter((p) => p.waiting).length, 2);

  const signal = { aborted: false };
  const stopping = { async post() { signal.aborted = true; return { sent: 1, failed: 0, skipped: 0, failures: [], remaining: 9, next: "k" }; } };
  const stopped = await runConfirmations(stopping, { signal });
  assert.deepEqual([stopped.sent, stopped.done], [1, false]);

  const broken = { async post() { throw Object.assign(new Error("no"), { kind: "config" }); } };
  await assert.rejects(runConfirmations(broken), (e: { totals: { batches: number } }) => e.totals.batches === 0);
  const limited = { async post() { throw Object.assign(new Error("rate"), { kind: "rate", retryAfter: 0 }); } };
  await assert.rejects(runConfirmations(limited, { maxWaits: 1, sleep: async () => {} }), /rate/);
  // the default sleep waits retry-after seconds
  let n = 0;
  const once = { async post() { if (n++ === 0) throw Object.assign(new Error("rate"), { kind: "rate", retryAfter: 0 }); return { sent: 0, failed: 0, skipped: 0, failures: [], remaining: 0, next: null }; } };
  assert.equal((await runConfirmations(once)).done, true);
});
