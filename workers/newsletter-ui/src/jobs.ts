// Background work: the cron trigger and the queue consumer. The send engine
// (GO-126, src/engine-*.ts) is registered here: its planner runs on every
// tick, its batch consumer handles SEND_JOB.
//
// Extending:
//   - cron: append a task to CRON_TASKS; each runs on every tick (every minute,
//     see [triggers] in wrangler.toml). A failing task is logged and does not
//     stop the others.
//   - queue: register a handler in JOB_HANDLERS under the job's `type`. A
//     handler that throws gets the message retried (with backoff), and after
//     max_retries it lands in the dead-letter queue, where it stays visible
//     instead of vanishing. A message with no handler is logged and acked:
//     retrying cannot make an unknown type known.

import { planTick } from "./engine-plan";
import { SEND_JOB } from "./engine-rate";
import { sendBatch } from "./engine-send";
import type { Env } from "./lib";
import { ensureSchema } from "./schema";

export interface Job {
  type: string;
  [key: string]: unknown;
}

export type CronTask = (env: Env, controller: ScheduledController) => Promise<void>;
export type JobHandler = (job: Job, env: Env) => Promise<void>;

export const CRON_TASKS: CronTask[] = [(env) => planTick(env)];
export const JOB_HANDLERS: Record<string, JobHandler> = {
  [SEND_JOB]: (job, env) => sendBatch(job, env),
};

export async function handleScheduled(
  controller: ScheduledController,
  env: Env,
  tasks: CronTask[] = CRON_TASKS,
): Promise<void> {
  if (tasks.length === 0) return; // nothing to do yet; don't touch D1 every minute
  await ensureSchema(env);
  for (const task of tasks) {
    try {
      await task(env, controller);
    } catch (e) {
      console.error("newsletter-ui: cron task failed", controller.cron, e);
    }
  }
}

export async function handleQueue(
  batch: MessageBatch<Job>,
  env: Env,
  handlers: Record<string, JobHandler> = JOB_HANDLERS,
): Promise<void> {
  for (const msg of batch.messages) {
    const type = msg.body && typeof msg.body.type === "string" ? msg.body.type : "";
    const handler = Object.hasOwn(handlers, type) ? handlers[type] : undefined;
    if (!handler) {
      console.warn("newsletter-ui: no handler for job", JSON.stringify(type), "on", batch.queue);
      msg.ack();
      continue;
    }
    try {
      await handler(msg.body, env);
      msg.ack();
    } catch (e) {
      console.error("newsletter-ui: job failed", type, "attempt", msg.attempts, e);
      msg.retry({ delaySeconds: Math.min(60 * 2 ** (msg.attempts - 1), 3600) });
    }
  }
}
