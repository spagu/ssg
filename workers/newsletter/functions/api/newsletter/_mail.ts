// Double opt-in confirmation mail. A leading underscore keeps this file out of
// the Pages route table — it is imported, never served.
//
// The transport (Resend, MailChannels or your own SMTP server) is chosen in
// _transport.ts. Called through waitUntil, so delivery never delays the
// visitor's response; failures are logged, never thrown.

import type { Env } from "./_lib";
import { STRINGS } from "./_i18n";
import { markConfirmSent } from "./_store";
import { sendMail, transportOf } from "./_transport";

export async function sendConfirmation(env: Env, to: string, lang: string, confirmURL: string): Promise<void> {
  if (!env.NEWSLETTER_MAIL_FROM || !transportOf(env)) return;
  const t = STRINGS[lang] || STRINGS.en;
  const result = await sendMail(env, { to, subject: t.mailSubject, text: t.mailBody.replace("{url}", confirmURL) });
  if (!result.ok) {
    console.warn(`newsletter mail: ${result.detail}`);
    return;
  }
  // Only a mail that actually left starts the re-send throttle.
  await markConfirmSent(env, to);
}
