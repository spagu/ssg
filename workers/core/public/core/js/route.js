// Which view the page shows, from the address's fragment: the home page, the
// read-only Appearance view (GO-152, /core/#appearance) or the module
// management (GO-149, /core/#modules). Anything else is the home page.
export const ROUTES = { "#appearance": "appearance", "#modules": "modules" };
export const routeOf = (hash) => (Object.hasOwn(ROUTES, hash) ? ROUTES[hash] : "home");
