// GET /api/shop/ucp/orders/:id — Get Order (order.md), for orders a UCP
// checkout placed; behind the same API key as checkout (_ucp-get-order.ts).

import { getUcpOrder } from "../../_ucp-get-order";
import { ucpRoute } from "../../_ucp-route";

export const onRequestGet = ucpRoute(getUcpOrder, { mutates: false });
