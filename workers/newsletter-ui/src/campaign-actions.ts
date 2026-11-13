// Campaign actions (GO-125): preview, test send, schedule, and the plain state
// moves (unschedule, pause, resume, cancel). The send engine (GO-126) acts on
// the states these set; nothing here sends to the list.

import { takeTestSend } from "./campaign-limit";
import { rendererFor } from "./click-track";
import { loadCampaign, readJson, senderOf } from "./campaign-model";
import type { Campaign } from "./campaign-model";
import { canTransition, transition } from "./campaign-state";
import type { Status } from "./campaign-state";
import type { Env } from "./lib";
import { json } from "./lib";
import { isAddress } from "./mail/_mime";
import { sendMail, transportOf } from "./mail/_transport";
import { transportReady } from "./transport-ready";
import { renderMessage, siteOf, unknownVariables, unsubscribeUrl } from "./render";
import type { Rendered, Site } from "./render";
import type { RouteContext } from "./router";
import { countSegment } from "./segment";

// Preview and test mails link to an unsubscribe token no subscriber has: the
// `newsletter` endpoint answers "this link is not valid", and a test address
// that happens to be on the list is never unsubscribed by trying the link.
export const PREVIEW_TOKEN = "preview-not-a-subscriber-token";
const LANGUAGE = /^[A-Za-z]{2,3}(-[A-Za-z0-9]{2,8})?$/;

const notFound = () => json({ error: "not_found" }, 404);
const noSite = () =>
  json({ error: "no_site_url", detail: "set NEWSLETTER_SITE_URL to the site's https:// origin (where the newsletter template runs)" }, 503);

// previewMessage is what both the preview and the test send show: the real
// renderer, for a recipient in `language` with the preview token.
export const previewMessage = (c: Campaign, language: string, site: Site): Rendered =>
  renderMessage(c, { email: "", language, token: PREVIEW_TOKEN }, site);

// shownMessage is the preview as the preview route and the test send show it:
// previewMessage, plus the click-counting links a recipient would get (GO-129,
// when on). `register` stores those links, so a test mail's links work.
async function shownMessage(env: Env, c: Campaign, language: string, site: Site, register: boolean): Promise<Rendered> {
  const r = { email: "", language, token: PREVIEW_TOKEN };
  return (await rendererFor(env, c, site, [r], register))(r);
}

const pickLanguage = (raw: unknown): string | null => (raw === undefined ? "en" : typeof raw === "string" && LANGUAGE.test(raw) ? raw : null);

// POST /api/campaigns/:id/preview {language?} → {subject, html, text, headers, recipients}
export async function previewCampaign({ request, env, params }: RouteContext): Promise<Response> {
  const b = (await readJson(request)) ?? {};
  const language = pickLanguage(b.language);
  if (!language) return json({ error: "invalid_input", detail: "language must be a language code" }, 400);
  const c = await loadCampaign(env, params.id);
  if (!c) return notFound();
  const site = siteOf(env);
  if (!site) return noSite();
  return json({ ...(await shownMessage(env, c, language, site, false)), recipients: await countSegment(env, c.segment) });
}

// POST /api/campaigns/:id/test {to, language?} — the preview mail, subject
// prefixed "[TEST] ", sent to one address through the configured transport.
export async function testCampaign({ request, env, params }: RouteContext): Promise<Response> {
  const b = await readJson(request);
  if (!b || typeof b.to !== "string" || !isAddress(b.to)) return json({ error: "invalid_to", detail: "to must be an email address" }, 400);
  const language = pickLanguage(b.language);
  if (!language) return json({ error: "invalid_input", detail: "language must be a language code" }, 400);
  const c = await loadCampaign(env, params.id);
  if (!c) return notFound();
  const ready = readiness(c, env);
  if (ready instanceof Response) return ready;
  if (!(await takeTestSend(env))) return json({ error: "test_limit", detail: "at most 10 test sends per hour" }, 429, { "retry-after": "3600" });
  const m = await shownMessage(env, c, language, ready.site, true);
  const result = await sendMail(env, { to: b.to, subject: `[TEST] ${m.subject}`, text: m.text, html: m.html, headers: m.headers, ...ready.sender });
  if (!result.ok) return json({ error: "send_failed", detail: result.detail, temporary: result.temporary }, 502);
  return json({ ok: true, transport: transportOf(env) });
}

// readiness checks what any real mail needs: a site URL, a transport whose
// settings can work, a sender.
function readiness(c: Campaign, env: Env): { site: Site; sender: { from: string; fromName: string } } | Response {
  const site = siteOf(env);
  if (!site) return noSite();
  const unusable = transportReady(env);
  if (unusable) return unusable;
  const sender = senderOf(c, env);
  if (!sender) return json({ error: "no_sender", detail: "set fromEmail on the campaign or NEWSLETTER_MAIL_FROM" }, 400);
  return { site, sender };
}

const invalidTransition = (from: Status, to: Status) => json({ error: "invalid_transition", detail: `${from} → ${to}` }, 409);

// hasUnsubscribe proves a rendered mail carries the unsubscribe link in both
// parts and in List-Unsubscribe. The renderer always adds it; checking the
// mail as it would go out means a future change cannot drop it silently.
export const hasUnsubscribe = (m: Rendered, url: string): boolean =>
  m.html.includes(url) && m.text.includes(url) && m.headers["List-Unsubscribe"] === `<${url}>`;

// contentProblem returns the first reason the campaign's content cannot be
// sent, or null. The checks run in order and lazily: rendering happens only
// once the variables are known to be valid (the renderer refuses unknown ones).
export function contentProblem(c: Campaign, site: Site): { error: string; detail: string } | null {
  const unknown = unknownVariables(c.subject, c.body, c.textOverride);
  const checks: [() => boolean, string, string][] = [
    [() => !c.subject.trim(), "empty_subject", "the campaign has no subject"],
    [() => !c.body.trim(), "empty_body", "the campaign has no body"],
    [() => unknown.length > 0, "unknown_variable", unknown.map((n) => `{{${n}}}`).join(", ")],
    [() => !hasUnsubscribe(previewMessage(c, "en", site), unsubscribeUrl(site, PREVIEW_TOKEN)), "missing_unsubscribe", "the rendered mail has no unsubscribe link"],
  ];
  const bad = checks.find(([fails]) => fails());
  return bad ? { error: bad[1], detail: bad[2] } : null;
}

// POST /api/campaigns/:id/schedule {at?} — validates everything a send needs,
// then draft → scheduled. `at` omitted or in the past = now.
export async function scheduleCampaign({ request, env, params }: RouteContext): Promise<Response> {
  const b = (await readJson(request)) ?? {};
  let at = new Date();
  if (b.at !== undefined) {
    const t = typeof b.at === "string" ? Date.parse(b.at) : Number.NaN;
    if (Number.isNaN(t)) return json({ error: "invalid_input", detail: "at must be an ISO 8601 time" }, 400);
    if (t > at.getTime()) at = new Date(t);
  }
  const c = await loadCampaign(env, params.id);
  if (!c) return notFound();
  if (!canTransition(c.status, "scheduled")) return invalidTransition(c.status, "scheduled");
  const ready = readiness(c, env);
  if (ready instanceof Response) return ready;
  const problem = contentProblem(c, ready.site);
  if (problem) return json(problem, 400);
  const recipients = await countSegment(env, c.segment);
  if (recipients.total === 0) return json({ error: "empty_segment", detail: "no confirmed subscriber matches the segment" }, 400);
  if (!(await transition(env, c.id, c.status, "scheduled", at.toISOString()))) return invalidTransition(c.status, "scheduled");
  return json({ ...(await loadCampaign(env, c.id)), recipients });
}

// move builds a handler that only changes state: unschedule, pause, resume,
// cancel. `from` lists the states the action applies to — narrower than the
// table where the engine shares a target: "resume" must not start a merely
// scheduled campaign (scheduled → sending is the engine's move).
export function move(to: Status, from: readonly Status[]) {
  return async ({ env, params }: RouteContext): Promise<Response> => {
    const c = await loadCampaign(env, params.id);
    if (!c) return notFound();
    if (!from.includes(c.status) || !(await transition(env, c.id, c.status, to))) return invalidTransition(c.status, to);
    return json(await loadCampaign(env, c.id));
  };
}

export const unscheduleCampaign = move("draft", ["scheduled"]);
export const pauseCampaign = move("paused", ["sending"]);
const resumeMove = move("sending", ["paused"]);

// resumeCampaign also clears the reason the engine paused it for
// (send_plans.error): after a resume it is history, not the current state.
// Cleared only while the campaign is `sending`, so a pause the engine makes
// again right away keeps its new reason if it lands first.
export async function resumeCampaign(ctx: RouteContext): Promise<Response> {
  const res = await resumeMove(ctx);
  if (res.ok) {
    await ctx.env.NEWSLETTER_DB.prepare(
      "UPDATE send_plans SET error = NULL WHERE campaign_id = ? AND error IS NOT NULL AND EXISTS (SELECT 1 FROM campaigns WHERE id = ? AND status = 'sending')",
    )
      .bind(ctx.params.id, ctx.params.id)
      .run();
  }
  return res;
}
export const cancelCampaign = move("cancelled", ["scheduled", "sending", "paused"]);
