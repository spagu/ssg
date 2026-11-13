// Long-running work in the browser: polling a sending campaign, and the loop
// that mails confirmations batch by batch. Timers and visibility are injected,
// so both are tested without a browser or real time.

// createPoller calls `tick` every `interval` ms until it resolves true (done)
// or stop() is called. While the tab is hidden it does not call the API at all
// (a campaign can send for hours; a background tab must not keep polling); it
// resumes with an immediate tick when shown again — wire `visible()` to
// document.visibilityState and call poller.wake() on visibilitychange.
export function createPoller({ tick, interval, visible = () => true, timers = globalThis, onError = () => {} }) {
  let timer = null;
  let stopped = false;
  let running = false;

  const schedule = () => {
    if (!stopped) timer = timers.setTimeout(run, interval);
  };

  async function run() {
    timer = null;
    if (stopped || running) return;
    if (!visible()) return; // wake() restarts it
    running = true;
    try {
      if (await tick()) stopped = true;
    } catch (e) {
      onError(e);
    } finally {
      running = false;
    }
    schedule();
  }

  return {
    start: () => run(),
    wake() {
      if (stopped || timer !== null || running || !visible()) return;
      void run(); // run() reports tick failures through onError
    },
    stop() {
      stopped = true;
      if (timer !== null) timers.clearTimeout(timer);
      timer = null;
    },
    get stopped() {
      return stopped;
    },
  };
}

// runConfirmations calls POST /api/subscribers/confirmations until `next` is
// null, adding up what each batch did. A 429 (the send rate's minute is used
// up) waits the retry-after and tries the same batch again. `signal.aborted`
// stops between batches; the totals so far are returned either way.
export async function runConfirmations(api, { onProgress = () => {}, signal = { aborted: false }, sleep, maxWaits = 30 } = {}) {
  const wait = sleep ?? ((s) => new Promise((r) => setTimeout(r, s * 1000)));
  const total = { sent: 0, failed: 0, skipped: 0, failures: [], remaining: null, batches: 0, done: false, waits: 0 };
  let after = null;
  while (!signal.aborted) {
    let r;
    try {
      r = await api.post("subscribers/confirmations", after ? { after } : {});
    } catch (e) {
      if (e.kind !== "rate" || total.waits >= maxWaits) throw Object.assign(e, { totals: total });
      total.waits++;
      onProgress({ ...total, waiting: e.retryAfter ?? 60 });
      await wait(e.retryAfter ?? 60);
      continue;
    }
    total.sent += r.sent;
    total.failed += r.failed;
    total.skipped += r.skipped;
    total.failures.push(...r.failures);
    total.remaining = r.remaining;
    total.batches++;
    after = r.next;
    if (!after) total.done = true;
    onProgress({ ...total, waiting: 0 });
    if (total.done) break;
  }
  return total;
}
