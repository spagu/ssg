// The audit log's vocabulary: every event kind the worker writes (_audit.ts
// EventKind), grouped for the filter, and an event's detail as readable
// pairs. Pure functions.

import { byCodeUnit } from "./perms.js";

export const EVENT_GROUPS = {
  signin: ["signin", "signin_refused", "invitation_accepted", "account_created", "groups_synced"],
  methods: ["method_linked", "method_unlinked", "password_set", "reset_link_issued"],
  factor: ["mfa_enrolled", "mfa_removed", "mfa_passed", "mfa_failed", "mfa_backup_code_used", "mfa_backup_codes_regenerated", "mfa_reset"],
  admin: ["account_blocked", "account_unblocked", "account_deleted", "grant_added", "grant_removed", "member_added", "member_removed", "role_created", "role_changed", "role_deleted", "group_deleted", "access_mapping_added", "access_mapping_removed", "invitation_sent", "invitation_revoked"],
  sessions: ["sessions_revoked", "session_revoked"],
  machines: ["service_token_created", "service_token_revoked", "signing_key_seen"],
};

export const EVENT_KINDS = Object.values(EVENT_GROUPS).flat();

// Detail keys that are shown elsewhere in the row (the administrator) or
// mean nothing to a reader (ids the row already links).
const HIDDEN = new Set(["by"]);

// detailPairs: an event's detail as [key, text] pairs, in a stable order.
export function detailPairs(detail) {
  if (!detail || typeof detail !== "object") return [];
  return Object.keys(detail)
    .filter((k) => !HIDDEN.has(k) && detail[k] !== null && detail[k] !== undefined && detail[k] !== "")
    .sort(byCodeUnit)
    .map((k) => {
      const v = detail[k];
      return [k, typeof v === "object" ? JSON.stringify(v) : String(v)];
    });
}
