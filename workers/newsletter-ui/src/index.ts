// newsletter-ui — the admin side of the newsletter: panel, admin API, and the
// cron + queue a send engine needs. A standalone Worker (`wrangler deploy` from
// this directory) sharing the D1 database NEWSLETTER_DB with the `newsletter`
// template. This file only wires the three entry points to their modules.

import { handleFetch } from "./app";
import { handleDeadLetters, isDeadLetterQueue } from "./engine-status";
import { handleQueue, handleScheduled } from "./jobs";
import type { Job } from "./jobs";
import type { Env } from "./lib";

export default {
  fetch(request: Request, env: Env, ctx: ExecutionContext): Promise<Response> {
    return handleFetch(request, env, ctx);
  },
  scheduled(controller: ScheduledController, env: Env, ctx: ExecutionContext): void {
    ctx.waitUntil(handleScheduled(controller, env));
  },
  // Two queues reach this consumer: send jobs, and the dead-letter queue
  // where a job lands after max_retries (recorded for the panel's count).
  queue(batch: MessageBatch<Job>, env: Env): Promise<void> {
    return isDeadLetterQueue(batch.queue, env) ? handleDeadLetters(batch, env) : handleQueue(batch, env);
  },
};
