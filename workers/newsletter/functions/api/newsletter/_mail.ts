// Double opt-in confirmation mail. A leading underscore keeps this file out of
// the Pages route table — it is imported, never served.
//
// Two providers, mirroring the contact-form template: Resend when
// NEWSLETTER_RESEND_API_KEY is set, otherwise the MailChannels Email API with
// NEWSLETTER_MAILCHANNELS_API_KEY. Called through waitUntil, so delivery never
// delays the visitor's response; failures are logged, never thrown.

import { Env } from "./_lib";
import { STRINGS } from "./_i18n";
import { markConfirmSent } from "./_store";

export async function sendConfirmation(env: Env, to: string, lang: string, confirmURL: string): Promise<void> {
  const from = env.NEWSLETTER_MAIL_FROM;
  if (!from) return;
  const t = STRINGS[lang] || STRINGS.en;
  const subject = t.mailSubject;
  const text = t.mailBody.replace("{url}", confirmURL);
  const name = env.NEWSLETTER_MAIL_FROM_NAME || "";

  let res: Response;
  try {
    if (env.NEWSLETTER_RESEND_API_KEY) {
      res = await fetch("https://api.resend.com/emails", {
        method: "POST",
        headers: { authorization: `Bearer ${env.NEWSLETTER_RESEND_API_KEY}`, "content-type": "application/json" },
        body: JSON.stringify({ from: name ? `${name} <${from}>` : from, to, subject, text }),
      });
    } else if (env.NEWSLETTER_MAILCHANNELS_API_KEY) {
      res = await fetch("https://api.mailchannels.net/tx/v1/send", {
        method: "POST",
        headers: { "x-api-key": env.NEWSLETTER_MAILCHANNELS_API_KEY, "content-type": "application/json" },
        body: JSON.stringify({
          personalizations: [{ to: [{ email: to }] }],
          from: name ? { email: from, name } : { email: from },
          subject,
          content: [{ type: "text/plain", value: text }],
        }),
      });
    } else {
      return;
    }
  } catch (e) {
    console.warn(`newsletter mail: ${e}`);
    return;
  }
  if (!res.ok) {
    console.warn(`newsletter mail: provider returned ${res.status}`);
    return;
  }
  // Only a mail that actually left starts the re-send throttle.
  await markConfirmSent(env, to);
}
