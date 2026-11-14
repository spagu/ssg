// Step-up from the panel (GO-140, GO-138): an action the API refused with
// 403 step_up_required waits while the person passes their second factor on
// /auth/mfa/step-up, which sends the browser back to the same view. The
// action is NEVER replayed: back in the panel the person sees that nothing
// was done yet and repeats it themselves — a destructive click must be made
// with the factor fresh, knowingly, not by a page that remembered it.
//
// The flow as states: idle → needed (a refusal, the dialog explains) →
// leaving (on the way to the step-up page) → returned (a notice) → idle.
// cancel goes back to idle from needed.

export const MARK = "stepped";

const TRANSITIONS = {
  idle: { refused: "needed" },
  needed: { go: "leaving", cancel: "idle", refused: "needed" },
  leaving: {},
  returned: { dismiss: "idle", refused: "needed" },
};

// next: the state after `event`; an event a state does not expect changes nothing.
export const next = (state, event) => TRANSITIONS[state]?.[event] ?? state;

// withMark adds stepped=1 to a hash route's query ("#/people/x?y=1").
export function withMark(hash) {
  const h = hash && hash.startsWith("#/") ? hash : "#/";
  return h + (h.includes("?") ? "&" : "?") + `${MARK}=1`;
}

// stepUpHref: the step-up page, coming back to `path` + the marked hash.
export function stepUpHref(stepUpUrl, path, hash) {
  const back = path + withMark(hash);
  return `${stepUpUrl}${stepUpUrl.includes("?") ? "&" : "?"}next=${encodeURIComponent(back)}`;
}

// arrival reads a hash on load: whether it came back from step-up, and the
// hash without the marker (to put back in the address bar).
export function arrival(hash) {
  const h = hash || "";
  const cut = h.indexOf("?");
  if (cut < 0) return { returned: false, hash: h };
  const q = new URLSearchParams(h.slice(cut + 1));
  if (q.get(MARK) !== "1") return { returned: false, hash: h };
  q.delete(MARK);
  const rest = q.toString();
  return { returned: true, hash: h.slice(0, cut) + (rest ? "?" + rest : "") };
}

// initial: the state the panel starts in for this address.
export const initial = (hash) => (arrival(hash).returned ? "returned" : "idle");
