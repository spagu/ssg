# Importing a subscriber list

`POST {base}/api/import/preview` shows what an import would do and writes
nothing; `POST {base}/api/import` does it. Upload a list exported from any
common mailing tool as it is: nobody has to know or edit its columns. Back to
the [README](../README.md) · [API reference](API.md).

The body is either the file (`content-type: text/csv`, options in the query:
`?consentSource=…&confirmed=true&dateOrder=dmy&mapping={"email":"…"}`, the
mapping as URL-encoded JSON) or JSON
`{"csv": "…", "consentSource": "…", "confirmed": true, "dateOrder": "dmy", "mapping": {…}}`.

## What is detected

(`src/import-read.ts`, `src/import-columns.ts`, `src/import-values.ts`)

- **Delimiter**: comma, semicolon or tab — whichever occurs most often outside
  quotes on the first line. **Encoding**: UTF-8 (BOM or not), or UTF-16 little
  or big endian with its BOM (a raw `text/csv` upload). CRLF, LF, quoted fields
  with `""` escapes and line breaks are fine.
- **Header row or none**: when the first cell of line 1 is already an address,
  the file has no header row and the address is the first column
  (single-column lists); the columns are then called `column 1`, `column 2`, ….
- **Columns**, by header name with case, spaces, `_`, `-` and `.` ignored, from
  one alias table (`ALIASES` in `src/import-columns.ts`):

  | Role | Typical headers |
  |---|---|
  | `email` (required) | Email, E-mail, Email Address, Mail, Subscriber Email, Contact Email |
  | `status` | Status, Subscriber Status, Member Status, Email Status, State |
  | `signedUpAt` | Subscribed At, Date Subscribed, Opt-in Time, Confirm Time, Signup Date, Signed up, Created At, Date Added, Joined, Timestamp |
  | `unsubscribedAt` | Unsubscribed At, Unsubscribe Date, Unsub Time, Opt-out Time |
  | `bouncedAt` | Bounced At, Bounce Date, Cleaned At, Clean Time |
  | `language` | Language, Lang, Locale (`en_US` is read as `en-US`) |
  | `tags` | Tags, Groups, Segments, Lists, Labels — split on `,` or `;` inside the cell |
  | `sourcePage` | Source Page, Signup Source, Source, Referrer, Signup URL |

  First name, last name, name, full name and the like are recognised and
  reported as *ignored: this worker does not store names* — data minimisation,
  they are read nowhere and stored nowhere. Every other column is listed as
  ignored. When two columns claim one role (an opt-in time and a creation
  time), the earlier alias in the table wins and `detected.ambiguous` says
  which column was used and which was not.
- **Override**: `mapping` sets a role's column by header name
  (`{"email": "Contact", "tags": null}`; `null` switches a role off, e.g. a
  tags column whose values the form would refuse). Overridden roles skip
  detection; the rest are still detected.

## Status values

Any case, `_ - .` read as spaces:

| In the file | Becomes |
|---|---|
| subscribed, active, confirmed, enabled, opted in, verified | active: `confirmed` with `consentSource` **and** `confirmed: true`, otherwise `pending` |
| pending, unconfirmed, not confirmed, unverified | `pending` |
| unsubscribed, opted out, removed, inactive, disabled, deleted, archived | inserted as `unsubscribed` |
| bounced, cleaned, invalid, hard bounce, undeliverable | suppression `hard` |
| complained, complaint, spam, marked as spam, abuse | suppression `complaint` |
| anything else | the row is `invalid_status`, listed with its value |
| (no status column, or an empty cell) | a filled bounce date means bounced, else a filled unsubscribe date means unsubscribed, else active |

A date cell holding `0`, `false`, `no`, `null`, `none`, `never`, `n/a` or `-`
counts as empty.

## What each row does

The rule stays absolute: **an import never undoes an unsubscribe and never
touches an existing row.** Precedence per row: invalid, then a repeat earlier
in the file (`duplicate`), then on the suppression list (`suppressed`:
bounced, complained, erased — skipped), then already in the table in any
status (`existing` — skipped, never updated), then — for a new address — by
status (list below).

An **existing** subscriber (any status) whom the file marks **bounced or
complained** keeps their row exactly as it is, but the address is
suppressed — `hard` / `complaint`, source `import`, address kept
(`toSuppressExisting`) — unless a suppression at least as strong is already
there (`complaint` > `hard` > `soft` = `manual`). The import never
unsubscribes anyone: the suppression alone stops mail, at the engine's last
check. Such rows still count as `existing` (or `suppressed`) in `counts`.

An address that is **not** in the table but **already suppressed for a weaker
reason** (`soft`, `manual`) and that the file marks bounced or complained has
its suppression raised to the file's reason (`toStrengthen`); it still counts
as `suppressed` in `counts`, and no subscriber row is written for it.

A new address, by status:

- **active** → `confirmed` or `pending` as above (`toConfirm` / `toPending`);
- **pending** → `pending` (`toPending`);
- **unsubscribed** → inserted as `unsubscribed`, `unsubscribed_at` from the
  file or now (`toUnsubscribed`), so a later import of an older copy of the
  list lists them as `existing` and cannot add them back as active. Needs no
  consent source; without one `consent_text` is "Imported as unsubscribed"
  (they are never mailed);
- **bounced / complaint** → never a subscriber row: a suppression with reason
  `hard` / `complaint`, source `import`, address kept (`toSuppress`).

`consentSource` is required when the import adds anyone who may be mailed
(active or pending rows), and also when it would write nothing at all;
writing only suppressions (for existing subscribers or raised ones) needs
none. It is stored as `consent_text`. **An import sends no mail by itself.**
Pending rows get the double opt-in confirmation mail only when an admin sends
it with `POST /api/subscribers/confirmations` (see the API reference); until
they confirm they are never mailed a campaign. Every row gets a new random
token of the template's shape, so its unsubscribe and confirm links work
there. `sourcePage` is kept only as a page path (a full URL keeps its path);
free text such as "landing form" is not stored, so `source_page` keeps the
template's shape. Each such dropped value is reported: `sourceWarnings`
counts them and `sourceWarningSample` lists the first 20 as `{line, value}`
(value cut at 100 characters). A placeholder (`-`, `none`, …, as for dates)
is an empty cell, not a warning.

## Dates

Accepted: ISO 8601 (`2024-03-04`, `2024-03-04T05:06:07Z`, with an offset),
`YYYY-MM-DD HH:MM[:SS]` (optionally `UTC`), Unix seconds or milliseconds,
`DD.MM.YYYY [HH:MM]`, `YYYY/MM/DD [HH:MM]`. A time without a zone is UTC.
**Slash dates** (`03/04/2024`) are read in one order per column: any value
whose first part is above 12 makes the column day-first, any whose second
part is above 12 month-first. If the column never decides (or contradicts
itself), nothing is guessed: preview and import answer
`400 ambiguous_date_format` with `columns` and `detected`, until the request
says `dateOrder: "dmy"` or `"mdy"`. Only the stored dates (`signedUpAt`,
`unsubscribedAt`) are read; a bounce date only implies the status. An
unreadable or future date is dropped, the row still imports, and the line is
listed in `dateWarnings`. A sign-up date fills `created_at` and `consent_at`
(and `confirmed_at` when the row becomes `confirmed`); without one, now.

## Response

The preview answers `{total, counts, sample, invalid, detected, classCounts,
classSample, dateWarnings, dateWarningCount, sourceWarnings,
sourceWarningSample}`; the import the same plus `imported` (subscriber rows
written), `suppressionsAdded` (every suppression written: new addresses,
existing subscribers and raised ones) and `status` (what active rows became).

- `counts` keeps its keys `new, duplicate, existing, suppressed, invalid`
  (`new` = every new address that will be written, of any class).
- `classCounts` splits `new` into `toConfirm, toPending, toUnsubscribed,
  toSuppress`, and adds `toSuppressExisting` (existing subscribers to
  suppress) and `toStrengthen` (suppressed addresses whose suppression the
  file raises) — neither is part of `new`. `classSample` lists up to 10 of
  each.
- `detected` is `{delimiter, headerless, headers, columns: {role: header |
  null}, ignored: [{header, code, reason}], ambiguous: [{role, chosen,
  alsoMatched}], dateOrder: {role: "dmy" | "mdy"}}`. `headers` is every
  header in file order (`column N` for a headerless file), for a mapping
  editor. Each ignored column has a machine `code` — `name_not_stored`,
  `used_elsewhere` (another column won the same role), `not_recognised` —
  next to the English `reason`. `dateOrder` is the order actually used for
  each date column: the request's `dateOrder` when given, else the one the
  column decided.

Extra errors: `400 invalid_mapping`, `400 invalid_date_order`,
`400 ambiguous_date_format`, `400 consent_source_required`,
`400 empty_file`, `400 missing_email_column`, `413 too_many_rows` /
`too_large`, `415 unsupported_media_type`.

## Worked example

A semicolon file saved by a spreadsheet:

```
E-mail;First name;Status;Date subscribed;Unsubscribed at;Groups
ana@example.com;Ana;Subscribed;03/04/2024;;"Weekly;Offers"
ben@example.com;Ben;Opted out;25/01/2023;02/02/2024;
cid@example.com;Cid;Cleaned;;;
dan@example.com;Dan;;;;news
```

The preview with `{"csv": …, "confirmed": true}` reports delimiter `;`,
`email` = "E-mail", `status` = "Status", `signedUpAt` = "Date subscribed",
`unsubscribedAt` = "Unsubscribed at", `tags` = "Groups", "First name" ignored
(names are not stored, `code: "name_not_stored"`), day-first dates
(`25/01/2023` decides). With a `consentSource` the import then writes Ana
`confirmed` (signed up 3 April 2024, tags `weekly,offers`), Ben
`unsubscribed` (unsubscribed 2 February 2024), Dan `confirmed` (signed up now,
tag `news`), and a `hard` suppression for Cid: `classCounts` =
`{toConfirm: 2, toPending: 0, toUnsubscribed: 1, toSuppress: 1,
toSuppressExisting: 0, toStrengthen: 0}`.

## Limits

`NEWSLETTER_IMPORT_MAX_ROWS` (default 20 000) rows, and 1 KiB per allowed row
of body. Lookups and inserts send each chunk of up to 1 000–2 000 rows as one
JSON parameter (`json_each`), so 10 000 rows of any mix cost about 20
statements, and all writes — subscribers and suppressions — run in one
`db.batch`: all rows or none.

## Suppression helpers for other code

`src/suppress.ts` exports `suppressionKey(email)`, `isSuppressed(env, email)`,
`findSuppression`, `suppress(env, {email, reason, source, keepEmail})`,
`suppressStatement` (the same upsert for a `db.batch`; repeats merge by
`SUPPRESSION_UPSERT`: `complaint` > `hard` > `soft` = `manual`, and an
address once dropped stays dropped), `unsuppress(env, emailHash)` and, for
queries over `subscribers`, `NOT_SUPPRESSED_SQL(alias)` +
`dropHashOnly(env, rows)`. D1 cannot compute SHA-256, so exclusion is two
steps: the SQL fragment drops addresses suppressed with the address kept
(indexed by `idx_suppressions_email`); `dropHashOnly` then hashes the
remaining rows in the worker against the hash-only (erased) keys — and
hashes nothing when there are none, the usual case. The fragment uses the
alias `sx` internally.
