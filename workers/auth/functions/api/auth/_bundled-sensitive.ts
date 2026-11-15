// The permissions the worker templates shipped with ssg declare
// `"sensitive": true` in their module.json (GO-181, GO-183). A leading
// underscore keeps this file out of the Pages route table.
//
// The registry (_registry.ts) learns sensitivity from AUTH_KNOWN_PERMISSIONS,
// which an operator writes by hand and may write as bare names — the format
// allows it, and Cloudflare's 5 KB cap on a variable pushes towards it. A bare
// name carries no flag, so before GO-183 designer-ui:theme listed that way was
// an ordinary permission and a service token carried it without an owner's
// allowSensitive. These names are sensitive whatever the variable says (fail
// closed); a test (test/bundled-sensitive.test.ts) keeps the list equal to the
// templates' manifests.
export const BUNDLED_SENSITIVE: readonly string[] = [
  "designer-ui:advanced", // settings that run code in CI or deploy, secret references
  "designer-ui:theme", // templates: code the build runs, HTML every visitor gets
  "editor-ui:publish", // raw HTML in Markdown reaches every visitor, at the panels' origin
];
