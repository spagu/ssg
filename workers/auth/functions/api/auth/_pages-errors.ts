// What the pages say for the codes the API puts in ?error= and ?ok=. A
// leading underscore keeps this file out of the Pages route table. A code the
// pages do not know shows the generic sentence — never the raw text of the
// query string, so a crafted link cannot put words on the page.

import { fill } from "./_i18n";
import type { Strings } from "./_i18n";

type Key = keyof Strings;

const ERRORS: Record<string, Key> = {
  invalid_credentials: "errInvalidCredentials",
  too_many_attempts: "errTooManyAttempts",
  turnstile_required: "errTurnstileRequired",
  invalid_request: "errInvalidRequest",
  permissions_too_large: "errPermissionsTooLarge",
  session_ended: "errSessionEnded",
  not_authenticated: "errSessionEnded",
  password_too_short: "errPasswordTooShort",
  password_too_long: "errPasswordTooLong",
  password_equals_email: "errPasswordEqualsEmail",
  password_required: "errPasswordRequired",
  invitation_invalid: "invalidInvite",
  reset_invalid: "invalidReset",
  account_exists: "errAccountExists",
  unsupported_method: "errUnsupportedMethod",
  too_many_requests: "errTooManyRequests",
  invalid_email: "errInvalidEmail",
  session_not_found: "errSessionNotFound",
  // GO-151: login providers and sign-in methods
  oauth_cancelled: "errOauthCancelled",
  oauth_state_invalid: "errOauthState",
  not_invited: "errNotInvited",
  email_unverified: "errEmailUnverified",
  provider_failed: "errProviderFailed",
  domain_not_allowed: "errDomainNotAllowed",
  identity_in_use: "errIdentityInUse",
  provider_already_linked: "errProviderAlreadyLinked",
  reauth_required: "errReauthRequired",
  method_last: "errMethodLast",
  method_not_found: "errMethodNotFound",
  method_disabled: "errMethodDisabled",
  password_exists: "errPasswordExists",
  account_blocked: "errAccountBlocked",
  // GO-140: the second factor
  invalid_code: "errInvalidCode",
  mfa_expired: "errMfaExpired",
  mfa_unavailable: "errMfaUnavailable",
  enrolment_expired: "errEnrolmentExpired",
  step_up_required: "errStepUpRequired",
  factor_required: "errFactorRequired",
  factor_not_found: "errFactorNotFound",
  no_factor: "errNoFactor",
  invalid_token: "errInvalidToken",
  not_owner: "errNotOwner",
  invalid_passkey: "errInvalidPasskey",
};

const OKS: Record<string, Key> = {
  signed_out: "okSignedOut",
  signed_out_all: "okSignedOutAll",
  password_reset: "okPasswordReset",
  password_changed: "okPasswordChanged",
  session_ended: "okSessionEnded",
  method_linked: "okMethodLinked",
  method_removed: "okMethodRemoved",
  password_set: "okPasswordSet",
  factor_added: "okFactorAdded",
  factor_removed: "okFactorRemoved",
};

export interface Rules {
  min: number;
  max: number;
  minutes: number;
}

// errorText: the sentence for an error code ("" = no code).
export function errorText(t: Strings, code: string | null, vars: Rules): string {
  if (!code) return "";
  return fill(t[Object.hasOwn(ERRORS, code) ? ERRORS[code] : "errGeneric"], { min: vars.min, max: vars.max, minutes: vars.minutes });
}

// okText: the sentence for a confirmation code; "" for an unknown one.
export function okText(t: Strings, code: string | null): string {
  return code && Object.hasOwn(OKS, code) ? t[OKS[code]] : "";
}

// PASSWORD_ERRORS belong to the new-password field.
export const PASSWORD_ERRORS = new Set(["password_too_short", "password_too_long", "password_equals_email", "password_required"]);
