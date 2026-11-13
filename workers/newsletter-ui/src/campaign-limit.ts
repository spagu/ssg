// The test-send limit: TEST_LIMIT test mails per hour for the whole worker.
//
// A test send goes to any address the admin types, so a stolen admin session
// (or a script with the password) could otherwise turn the worker into a spam
// relay on the site's own sending reputation. The limit is global rather than
// per address for the same reason.
//
// The counter lives in the `settings` table under one key, as
// {"since": ISO time, "count": n}: a window opens with the first test send and
// lasts an hour. Taking a slot is ONE upsert whose WHERE refuses the update
// when the window is full, so two concurrent requests cannot both take the
// last slot. A slot is taken before sending and not given back on failure:
// retrying a failing transport also counts.

import type { Env } from "./lib";

export const TEST_LIMIT = 10;
export const TEST_WINDOW_MS = 60 * 60 * 1000;
export const TEST_KEY = "campaign_test_sends";

// takeTestSend returns true when a slot was taken, false when the hour is used up.
export async function takeTestSend(env: Env, now = new Date()): Promise<boolean> {
  const at = now.toISOString();
  const cutoff = new Date(now.getTime() - TEST_WINDOW_MS).toISOString();
  const r = await env.NEWSLETTER_DB.prepare(
    `INSERT INTO settings (key, value, updated_at) VALUES (?1, json_object('since', ?2, 'count', 1), ?2)
     ON CONFLICT (key) DO UPDATE SET
       value = CASE WHEN json_extract(settings.value, '$.since') <= ?3
                    THEN json_object('since', ?2, 'count', 1)
                    ELSE json_object('since', json_extract(settings.value, '$.since'),
                                     'count', json_extract(settings.value, '$.count') + 1) END,
       updated_at = ?2
     WHERE json_extract(settings.value, '$.since') <= ?3 OR json_extract(settings.value, '$.count') < ?4`,
  )
    .bind(TEST_KEY, at, cutoff, TEST_LIMIT)
    .run();
  return r.meta.changes === 1;
}
