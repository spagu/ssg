// POST /testing/simulate-shipping/:id — the UCP conformance suite's hook for
// advancing an order (GO-187, _ucp-simulate.ts). Not part of the protocol: a
// 404 unless SHOP_UCP_TEST_MODE is "true" and SHOP_UCP_SIMULATION_SECRET is
// set, and Pages only routes it here when routes_include names /testing/*.

import { onSimulateShipping } from "../../api/shop/_ucp-simulate";

export const onRequestPost = onSimulateShipping;
