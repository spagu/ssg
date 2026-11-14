// The auth module's summary (GO-148 format, GO-138): accounts, open
// invitations, live sessions and accounts without a second factor their
// roles need; what needs a person; configuration problems. A leading
// underscore keeps this file out of the Pages route table —
// module/summary.ts serves it. Aggregates only: counts, never an address.

import { accessLookup } from "./_access-config";
import { keyAges } from "./_admin-keys";
import { missingFactor, pendingInvitations, rolesNeedingFactor } from "./_admin-status";
import type { Ctx } from "./_context";
import { iso } from "./_lib";
import { mailState } from "./_mail";
import { mfaState } from "./_mfa-crypto";
import { mfaOverrides } from "./_mfa-policy";
import { counted, summaryOf } from "./_summary-kit";
import type { Attention, CountedText, Metric, Problem, Summary, Text } from "./_summary-kit";

const PANEL = "/auth/admin/";

const LABELS: Record<"accounts" | "pendingInvitations" | "liveSessions" | "missingFactor", Text> = {
  accounts: { en: "Active accounts", pl: "Aktywne konta", hi: "सक्रिय खाते", de: "Aktive Konten", fr: "Comptes actifs" },
  pendingInvitations: { en: "Open invitations", pl: "Otwarte zaproszenia", hi: "खुले आमंत्रण", de: "Offene Einladungen", fr: "Invitations ouvertes" },
  liveSessions: { en: "Live sessions", pl: "Aktywne sesje", hi: "सक्रिय सेशन", de: "Aktive Sitzungen", fr: "Sessions actives" },
  missingFactor: { en: "Without a needed second factor", pl: "Bez wymaganego drugiego składnika", hi: "आवश्यक दूसरे फ़ैक्टर के बिना", de: "Ohne nötigen zweiten Faktor", fr: "Sans le second facteur requis" },
};

const INVITES: CountedText = {
  en: { one: "{count} invitation is still open.", other: "{count} invitations are still open." },
  pl: { one: "{count} zaproszenie jest wciąż otwarte.", few: "{count} zaproszenia są wciąż otwarte.", many: "{count} zaproszeń jest wciąż otwartych.", other: "{count} zaproszenia jest wciąż otwartego." },
  hi: { one: "{count} आमंत्रण अभी खुला है।", other: "{count} आमंत्रण अभी खुले हैं।" },
  de: { one: "{count} Einladung ist noch offen.", other: "{count} Einladungen sind noch offen." },
  fr: { one: "{count} invitation est encore ouverte.", other: "{count} invitations sont encore ouvertes." },
};
const MISSING: CountedText = {
  en: { one: "{count} account holds a role that needs a second factor but has none.", other: "{count} accounts hold a role that needs a second factor but have none." },
  pl: { one: "{count} konto ma rolę wymagającą drugiego składnika, ale go nie ma.", few: "{count} konta mają rolę wymagającą drugiego składnika, ale go nie mają.", many: "{count} kont ma rolę wymagającą drugiego składnika, ale go nie ma.", other: "{count} konta ma rolę wymagającą drugiego składnika, ale go nie ma." },
  hi: { one: "{count} खाते के पास ऐसी भूमिका है जिसे दूसरा फ़ैक्टर चाहिए, पर उसके पास कोई नहीं है।", other: "{count} खातों के पास ऐसी भूमिका है जिसे दूसरा फ़ैक्टर चाहिए, पर उनके पास कोई नहीं है।" },
  de: { one: "{count} Konto hat eine Rolle, die einen zweiten Faktor braucht, aber keinen.", other: "{count} Konten haben eine Rolle, die einen zweiten Faktor braucht, aber keinen." },
  fr: { one: "{count} compte a un rôle qui exige un second facteur, sans en avoir.", other: "{count} comptes ont un rôle qui exige un second facteur, sans en avoir." },
};
const KEY_OLD: CountedText = {
  en: { one: "The signing key is {count} day old: rotate it.", other: "The signing key is {count} days old: rotate it." },
  pl: { one: "Klucz podpisu ma {count} dzień: wymień go.", other: "Klucz podpisu ma {count} dni: wymień go." },
  hi: { one: "साइनिंग कुंजी {count} दिन पुरानी है: इसे बदलें।", other: "साइनिंग कुंजी {count} दिन पुरानी है: इसे बदलें।" },
  de: { one: "Der Signaturschlüssel ist {count} Tag alt: Wechseln Sie ihn.", other: "Der Signaturschlüssel ist {count} Tage alt: Wechseln Sie ihn." },
  fr: { one: "La clé de signature a {count} jour : remplacez-la.", other: "La clé de signature a {count} jours : remplacez-la." },
};

const PROBLEMS: Record<string, Text> = {
  mail_broken: {
    en: "A mail transport is set but can never send: invitations and reset links cannot go out by mail.",
    pl: "Ustawiono transport poczty, ale nigdy nie wyśle: zaproszenia i linki resetu nie wyjdą pocztą.",
    hi: "एक मेल ट्रांसपोर्ट सेट है पर कभी भेज नहीं सकता: आमंत्रण और रीसेट लिंक मेल से नहीं जा सकते।",
    de: "Ein Mail-Transport ist gesetzt, kann aber nie senden: Einladungen und Reset-Links gehen nicht per Mail hinaus.",
    fr: "Un transport de courriel est défini mais ne peut jamais envoyer : invitations et liens de réinitialisation ne partent pas par courriel.",
  },
  mfa_key_short: {
    en: "AUTH_MFA_KEY is shorter than 32 characters: authenticator-app codes can be neither set up nor checked.",
    pl: "AUTH_MFA_KEY ma mniej niż 32 znaki: kodów z aplikacji nie da się ani ustawić, ani sprawdzić.",
    hi: "AUTH_MFA_KEY 32 अक्षरों से छोटा है: ऑथेंटिकेटर ऐप के कोड न सेट हो सकते हैं, न जाँचे जा सकते हैं।",
    de: "AUTH_MFA_KEY ist kürzer als 32 Zeichen: Codes aus Authenticator-Apps können weder eingerichtet noch geprüft werden.",
    fr: "AUTH_MFA_KEY fait moins de 32 caractères : les codes d'application ne peuvent être ni configurés ni vérifiés.",
  },
  mfa_roles_invalid: {
    en: "AUTH_MFA_ROLES is not a JSON object of role names to true/false: every role needs a second factor until it is fixed.",
    pl: "AUTH_MFA_ROLES nie jest obiektem JSON nazw ról z true/false: do czasu poprawki każda rola wymaga drugiego składnika.",
    hi: "AUTH_MFA_ROLES भूमिका नामों से true/false का JSON ऑब्जेक्ट नहीं है: ठीक होने तक हर भूमिका को दूसरा फ़ैक्टर चाहिए।",
    de: "AUTH_MFA_ROLES ist kein JSON-Objekt aus Rollennamen und true/false: Bis zur Korrektur braucht jede Rolle einen zweiten Faktor.",
    fr: "AUTH_MFA_ROLES n'est pas un objet JSON de noms de rôles vers true/false : chaque rôle exige un second facteur jusqu'à correction.",
  },
  access_misconfigured: {
    en: "Cloudflare Access is configured by halves or wrongly, so it is not offered as a way in.",
    pl: "Cloudflare Access skonfigurowano połowicznie albo błędnie, więc nie jest oferowany jako sposób logowania.",
    hi: "Cloudflare Access आधा या ग़लत कॉन्फ़िगर है, इसलिए साइन इन के तरीक़े के रूप में नहीं दिया जाता।",
    de: "Cloudflare Access ist nur halb oder falsch konfiguriert und wird daher nicht als Anmeldeweg angeboten.",
    fr: "Cloudflare Access est configuré à moitié ou mal, il n'est donc pas proposé pour se connecter.",
  },
};

// authSummary is the answer of GET /api/auth/module/summary.
export async function authSummary(ctx: Ctx): Promise<Summary> {
  const { env, now } = ctx;
  const codes: string[] = [];
  if (mailState(env).kind === "broken") codes.push("mail_broken");
  if (mfaState(env) === "misconfigured") codes.push("mfa_key_short");
  if (mfaState(env) !== "off" && mfaOverrides(env) === "invalid") codes.push("mfa_roles_invalid");
  if (accessLookup(env).kind === "misconfigured") codes.push("access_misconfigured");
  const problems: Problem[] = codes.map((code) => ({ code, text: PROBLEMS[code] }));
  const row = (await env.AUTH_DB.prepare(
    `SELECT (SELECT COUNT(*) FROM users WHERE status = 'active') AS accounts,
            (SELECT COUNT(*) FROM sessions WHERE revoked_at IS NULL AND expires_at > ?) AS sessions`,
  )
    .bind(iso(now))
    .first<{ accounts: number; sessions: number }>()) as { accounts: number; sessions: number };
  const pending = await pendingInvitations(env, now);
  const missing = await missingFactor(env, await rolesNeedingFactor(env));
  const metrics: Metric[] = [
    { key: "accounts", label: LABELS.accounts, value: row.accounts },
    { key: "pendingInvitations", label: LABELS.pendingInvitations, value: pending },
    { key: "liveSessions", label: LABELS.liveSessions, value: row.sessions },
    { key: "missingFactor", label: LABELS.missingFactor, value: missing },
  ];
  const attention: Attention[] = [];
  const [current] = await keyAges(env, [ctx.keys.signing.kid], ctx.keys.signing.kid, now);
  if (current.rotate) attention.push({ severity: "warning", text: counted(KEY_OLD, current.ageDays as number), href: `${PANEL}#/status` });
  if (missing > 0) attention.push({ severity: "action", text: counted(MISSING, missing), href: `${PANEL}#/status` });
  if (pending > 0) attention.push({ severity: "info", text: counted(INVITES, pending), href: `${PANEL}#/invitations` });
  return summaryOf({ metrics, attention, problems }, new Date(now));
}
