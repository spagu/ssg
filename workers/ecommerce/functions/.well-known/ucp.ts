// GET /.well-known/ucp — the shop's UCP business profile (GO-167,
// _ucp-profile.ts). Public, as the protocol and Google require. Off (404)
// unless SHOP_UCP_ENABLED is "true". The site must route this path to
// Functions: add /.well-known/ucp to the worker's routes_include (UCP.md).

import { onProfileGet } from "../api/shop/_ucp-profile";

export const onRequestGet = onProfileGet;
