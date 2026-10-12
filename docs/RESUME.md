# Résumé from Schema Resume

ssg can publish a CV from a single JSON document in the
[Schema Resume](https://schema-resume.org) format. You keep one file,
`cv.json`, and the build turns it into three things:

- a page of your site, at `/cv/` by default, that renders in any theme;
- `ProfilePage` structured data in which the `Person` carries your CV;
- the document itself, published next to the page as `/cv/resume.json`.

Schema Resume is an open (MIT) format with sections for the person, their work,
volunteering, education, awards, certificates, publications, skills, tools,
languages, interests, references and projects. ssg reads version 1.3 of the
schema (`https://schema-resume.org/1.3/schema.json`). It accepts dates at year,
month or day precision (`2019`, `2019-06`, `2019-06-15`) and keeps any custom
field you add.

## Quick start

```yaml
resume:
  file: cv.json        # the Schema Resume document
  url: /cv/            # where the page goes (default /cv/)
  # title: Curriculum vitae   # default: basics.name
  # contact: true             # publish e-mail, phone and street address
seo: true              # lets ssg inject the structured data (see below)
```

A minimal `cv.json`:

```json
{
  "$schema": "https://schema-resume.org/1.3/schema.json",
  "basics": { "name": "Ada Kowalczyk", "label": "Platform Engineer" },
  "work": [
    { "name": "Example Logistics", "position": "Senior Platform Engineer", "startDate": "2021-03" }
  ]
}
```

[`examples/resume/`](../examples/resume/) contains a complete site:

```sh
ssg --config examples/resume/ssg.yaml
```

## Configuration

| Key | Default | Meaning |
|---|---|---|
| `resume.file` | (off) | Path to the JSON document, relative to where `ssg` runs. Without it, no résumé is built. |
| `resume.url` | `/cv/` | URL of the page. It is always treated as a directory: `/cv` becomes `/cv/`. |
| `resume.title` | `basics.name` | Page title. |
| `resume.contact` | `false` | Publish `basics.email`, `basics.phone` and the street address (`location.address`, `streetAddress`, `postalCode`). |

## What is generated

| Output | Content |
|---|---|
| `<url>index.html` | The CV as a page. The body is Markdown generated from the document, so it goes through the theme's `page.html` like any other page. It also appears in search, the sitemap, `llms.txt` and `markdown_publish`. |
| `<url>resume.json` | The document. With `contact: true` it is the file byte for byte. Otherwise the contact fields are removed and the JSON is re-encoded with sorted keys. |
| `<link rel="alternate" type="application/json" href="<url>resume.json">` | Added to the page's `<head>`, so tools can find the data. |
| JSON-LD | A `ProfilePage` with `mainEntity: Person`, produced by the normal SEO pipeline (`seo: true`). |

The page body has these sections, in this order. A section with no data is
left out.

1. Label (job title), summary, location, website and profile links
2. Key achievements, Core competencies
3. Experience: one heading per employer, and one per position when `positions` lists several
4. Volunteering
5. Education
6. Skills and Tools
7. Certificates and Languages
8. Projects
9. Publications, Awards, Interests, References

Dates are shown as written (`2019-06 – present`). Text from the document is
escaped, so `C#` or `<b>` is printed as typed and does not become markup. A
link is written only for an absolute `http(s)` URL. Section headings are in
English. To use other words or another layout, write a `resume.html`
template (see [Theming](#theming)).

The page has `.Page.Modified` set from `meta.dateModified` (or
`meta.lastModified`) when that is an RFC 3339 timestamp, and the sitemap uses
it.

## Structured data

The page's schema is a `ProfilePage` whose `mainEntity` is a `Person`. It is
built with the same Person helper as author pages:

| Schema Resume | schema.org |
|---|---|
| `basics.name` | `Person.name` |
| `basics.label` | `Person.jobTitle` |
| `basics.title` | `Person.honorificPrefix` |
| `basics.summary` | `Person.description` |
| `basics.url`, `basics.image` | `Person.url`, `Person.image` (absolute http(s) only) |
| `basics.profiles[].url` | `Person.sameAs` |
| `work[]` with no `endDate` (or an open position) | `Person.worksFor` → `Organization` (name, url) |
| `work[].position`, `work[].positions[].position` | `Person.hasOccupation` → `Occupation` (one per distinct title) |
| `education[].institution` / `school` | `Person.alumniOf` → `EducationalOrganization` |
| `certificates[]` | `Person.hasCredential` → `EducationalOccupationalCredential` (name, url, `dateCreated`, `recognizedBy` the issuer) |
| `skills[].name` + `skills[].keywords` | `Person.knowsAbout` |
| `languages[].language` | `Person.knowsLanguage` → `Language` |
| `awards[].title` | `Person.award` |
| `basics.email`, `basics.phone`, `basics.location` | `Person.email`, `telephone`, `address` (`PostalAddress`), **only with `contact: true`** |
| `meta.dateCreated`, `meta.dateModified` | `ProfilePage.dateCreated`, `dateModified` |

The block goes through ssg's usual merge. The site-wide `schema:` comes first,
then the derived `WebPage` data (name, url, description, image), then
`schema_defaults`, and the résumé's `ProfilePage` is applied last. Breadcrumbs
are added as on any other page. `check_schema` validates the result. None of
these types has required properties in its table, so a valid CV produces no
findings.

## Privacy

`contact` is off by default. In that state, the e-mail address, phone number,
street address and postal code do not appear in the page body, in the JSON-LD,
in `resume.json` or in `.Page.Extra.resume`. The city, region and country are
still shown.

Every other part of the document is published unchanged. This includes date
of birth, nationality, work authorisation, references and custom fields. If
something should stay private, leave it out of the file.

## Diagnostics

The document is checked at build time, without network access:

- a missing `basics.name` is an error and fails the build;
- a date that is not `YYYY`, `YYYY-MM` or `YYYY-MM-DD`, or that names a day
  that does not exist, produces a warning;
- an end date earlier than its start date produces a warning;
- a URL that is not an absolute `http(s)` URL produces a warning;
- an e-mail address that does not parse produces a warning;
- a `$schema` that does not point to schema-resume.org produces a warning.

Warnings are printed as `resume <file>: <path>: <message>`, for example
`work[2].startDate`. A file that is not JSON, or that has a field of the wrong
type (a string where the schema expects a list), fails the build.

## Theming

The page uses layout `resume`. When the theme has `resume.html` (or
`layouts/resume.html`), that template is used. Otherwise the theme's
`page.html` renders the generated Markdown.

In either template the parsed document is available as `.Page.Extra.resume`
(also `.resume`). Field names are Go field names, and `Raw` holds the whole
document, custom fields included. The URL of the JSON copy is
`.Page.Extra.resume_json`.

```html
{{ with .Page.Extra.resume }}
  <h1>{{ .Basics.Name }}</h1>
  <p>{{ .Basics.Label }}</p>
  {{ range .Work }}
    <h2>{{ .Position }} — {{ .Name }}</h2>
    <p>{{ .StartDate }} – {{ or .EndDate "present" }}</p>
  {{ end }}
  {{ index .Raw "pronouns" }}
{{ end }}
<a href="{{ .Page.Extra.resume_json }}">JSON</a>
```

When `contact` is off, the document a template receives already has the
contact fields removed.

## Limitations

- There is one résumé per site, and it has one language.
- Section headings in the generated body are in English. Use a `resume.html`
  layout for anything else.
- The document is not validated against the full JSON Schema. ssg applies the
  rules listed above and decodes the fields it knows. A wrongly typed known
  field fails the build, and unknown fields are kept.
- A resume file that changes during `--watch` triggers a full rebuild.
