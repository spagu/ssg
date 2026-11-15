// Who may call which admin route (GO-155). A leading underscore keeps this file
// out of the Pages route table.
//
// ADMIN_ROUTES is the one table: every route under /api/shop/admin/ and the
// single permission it needs. The admin middleware enforces it for every
// request, so a handler never checks a permission itself and cannot forget
// to. test/admin-routes.test.ts reads the route files under admin/ and fails
// for a route that is missing here, for an entry here without a route, and for
// a permission module.json does not declare. MODULE_ROUTES does the same for
// the two module routes (GO-156), which live outside admin/ but pass the same
// gate (_gate.ts). MACHINE_ROUTES (GO-157) lists the few an `auth` service
// token may call.
//
// The permissions mean something in `auth` mode only, where the session token
// carries them. Cloudflare Access and the shared password grant every one
// (_auth.ts), as in every other worker.

// PERMISSIONS: module.json's list, in its order.
export const PERMISSIONS = [
  "ecommerce:read", //     orders, products, stats, audit and outbox views
  "ecommerce:orders", //   resend links, mark paid, cancel, notes, retry queued mail
  "ecommerce:refund", //   refunds: money leaves the shop
  "ecommerce:products", // create and edit products, upload files, VAT rates
  "ecommerce:settings", // gateways and shop settings
  "ecommerce:export", //   exports that carry buyers' personal data
] as const;
export type Permission = (typeof PERMISSIONS)[number];

// SESSION: signed in is enough. Only `me` uses it — an account holding no shop
// permission at all must still learn who it is and what it lacks. Not a valid
// permission name (no colon), so it can never collide with one.
export const SESSION = "session";
export type Need = Permission | typeof SESSION;

// Keys are "<METHOD> <path>", with :name for a [name] segment of the file tree.
export const ADMIN_ROUTES: Readonly<Record<string, Need>> = {
  "GET /api/shop/admin/me": SESSION,
  "GET /api/shop/admin/stats": "ecommerce:read",
  "GET /api/shop/admin/audit": "ecommerce:read",
  "GET /api/shop/admin/outbox": "ecommerce:read",
  "POST /api/shop/admin/outbox": "ecommerce:orders",
  "GET /api/shop/admin/orders": "ecommerce:read",
  "GET /api/shop/admin/orders/:id": "ecommerce:read",
  "PATCH /api/shop/admin/orders/:id": "ecommerce:orders",
  "POST /api/shop/admin/orders/:id/resend": "ecommerce:orders",
  "POST /api/shop/admin/orders/:id/mark-paid": "ecommerce:orders",
  "POST /api/shop/admin/orders/:id/cancel": "ecommerce:orders",
  "POST /api/shop/admin/orders/:id/refund": "ecommerce:refund",
  "GET /api/shop/admin/products": "ecommerce:read",
  "POST /api/shop/admin/products": "ecommerce:products",
  "GET /api/shop/admin/products/:id": "ecommerce:read",
  "PATCH /api/shop/admin/products/:id": "ecommerce:products",
  "DELETE /api/shop/admin/products/:id": "ecommerce:products",
  "POST /api/shop/admin/products/:id/file": "ecommerce:products",
  "GET /api/shop/admin/vat-rates": "ecommerce:read",
  "POST /api/shop/admin/vat-rates": "ecommerce:products",
  "DELETE /api/shop/admin/vat-rates": "ecommerce:products",
  "GET /api/shop/admin/settings": "ecommerce:settings",
  "PUT /api/shop/admin/settings": "ecommerce:settings",
  "GET /api/shop/admin/gateways": "ecommerce:settings",
  "POST /api/shop/admin/gateways": "ecommerce:settings",
  "GET /api/shop/admin/export": "ecommerce:export",
};

// MODULE_ROUTES: the module's manifest and summary (GO-156). Outside admin/, so
// the admin middleware never runs for them; each handler calls the same gate
// with the permission found here.
export const MODULE_ROUTES: Readonly<Record<string, Permission>> = {
  "GET /api/shop/module": "ecommerce:read",
  "GET /api/shop/module/summary": "ecommerce:read",
};

// MACHINE_ROUTES: what an `auth` service token — a script, CI, a cron job, no
// person and no browser — may call (GO-157), and only while it holds the
// route's permission by its exact name (_gate.ts). Reads only, chosen one by
// one: the export (orders and invoices for bookkeeping), the sales figures
// and the module's own manifest and summary (monitoring). Nothing that
// changes the shop is here: a refund, a resend or a price change is a
// person's decision, recorded under their name. A route added to this list
// must be a GET unless the decision is made again and written down here —
// test/machine-tokens.test.ts holds the list to that.
export const MACHINE_ROUTES: readonly string[] = [
  "GET /api/shop/admin/export",
  "GET /api/shop/admin/stats",
  "GET /api/shop/module",
  "GET /api/shop/module/summary",
];

// matchRoute finds the key of `keys` that a request matches, or null. Keys are
// "<METHOD> <path>", :name standing for any one non-empty segment. HEAD is a
// GET without a body, as Pages serves it; a trailing slash is the same route.
export function matchRoute(keys: Iterable<string>, method: string, pathname: string): string | null {
  const verb = method === "HEAD" ? "GET" : method;
  const segments = pathname.replace(/\/+$/, "").split("/");
  for (const key of keys) {
    const [keyMethod, keyPath] = key.split(" ") as [string, string];
    if (keyMethod !== verb) continue;
    const pattern = keyPath.split("/");
    if (pattern.length !== segments.length) continue;
    if (pattern.every((p, i) => p === segments[i] || (p.startsWith(":") && segments[i] !== ""))) return key;
  }
  return null;
}

const ROUTES: Readonly<Record<string, Need>> = { ...ADMIN_ROUTES, ...MODULE_ROUTES };

// needFor answers what a request needs, or null for a route the tables do not
// know — which the gate refuses in every mode (fail closed).
export function needFor(method: string, pathname: string): Need | null {
  const key = matchRoute(Object.keys(ROUTES), method, pathname);
  return key === null ? null : ROUTES[key]!;
}

// machineSafe: whether a service token may call this route at all.
export const machineSafe = (method: string, pathname: string): boolean => matchRoute(MACHINE_ROUTES, method, pathname) !== null;
