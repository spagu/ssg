// Outgoing mail for the contact form. A leading underscore keeps this file out
// of the Pages route table — it is imported, never served.
//
// Two providers, mirroring the newsletter template (#379):
//   CONTACT_RESEND_API_KEY        Resend (https://resend.com)
//   CONTACT_MAILCHANNELS_API_KEY  MailChannels Email API, sent as X-Api-Key —
//                                 keyless sending from Workers ended in 2024
//   RESEND_API_KEY                legacy unprefixed Resend key, read only when
//                                 neither prefixed key is set
// One of them is required. The prefixed names win over the shared one because
// every worker in a Pages project sees the same environment (#325): a
// RESEND_API_KEY set for another template must not override the MailChannels
// key this form was given.

export interface MailEnv {
  CONTACT_TO?: string;
  CONTACT_FROM?: string;
  CONTACT_RESEND_API_KEY?: string;
  CONTACT_MAILCHANNELS_API_KEY?: string;
  RESEND_API_KEY?: string;
}

export interface Message {
  replyTo: { email: string; name: string };
  subject: string;
  text: string;
}

type Provider = { kind: "resend" | "mailchannels"; key: string };

// provider picks the mail service from the environment, or null when none is
// configured.
export function provider(env: MailEnv): Provider | null {
  if (env.CONTACT_RESEND_API_KEY) return { kind: "resend", key: env.CONTACT_RESEND_API_KEY };
  if (env.CONTACT_MAILCHANNELS_API_KEY) return { kind: "mailchannels", key: env.CONTACT_MAILCHANNELS_API_KEY };
  if (env.RESEND_API_KEY) return { kind: "resend", key: env.RESEND_API_KEY };
  return null;
}

// missingMailConfig names what keeps the form from sending, or returns "" when
// it can send. The caller answers 503 with this text.
export function missingMailConfig(env: MailEnv): string {
  if (!env.CONTACT_TO || !env.CONTACT_FROM) return "CONTACT_TO and CONTACT_FROM must be set";
  if (!provider(env)) return "set CONTACT_RESEND_API_KEY or CONTACT_MAILCHANNELS_API_KEY";
  return "";
}

// sendMail delivers msg to CONTACT_TO. It returns false — after logging — when
// no provider is configured, the request fails or the provider refuses it.
export async function sendMail(env: MailEnv, msg: Message): Promise<boolean> {
  const p = provider(env);
  if (!p) {
    console.error("contact mail: no provider key set");
    return false;
  }
  const req =
    p.kind === "resend"
      ? {
          url: "https://api.resend.com/emails",
          headers: { authorization: `Bearer ${p.key}`, "content-type": "application/json" },
          body: {
            from: env.CONTACT_FROM,
            to: env.CONTACT_TO,
            subject: msg.subject,
            text: msg.text,
            reply_to: msg.replyTo.email,
          },
        }
      : {
          url: "https://api.mailchannels.net/tx/v1/send",
          headers: { "x-api-key": p.key, "content-type": "application/json" },
          body: {
            personalizations: [{ to: [{ email: env.CONTACT_TO }] }],
            from: { email: env.CONTACT_FROM, name: "Website" },
            reply_to: msg.replyTo,
            subject: msg.subject,
            content: [{ type: "text/plain", value: msg.text }],
          },
        };
  try {
    const res = await fetch(req.url, { method: "POST", headers: req.headers, body: JSON.stringify(req.body) });
    if (res.ok) return true;
    console.error(`contact mail: ${p.kind} returned ${res.status}`);
  } catch (e) {
    console.error(`contact mail: ${p.kind} request failed: ${e}`);
  }
  return false;
}
