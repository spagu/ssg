// CSV export helpers. A leading underscore keeps this file out of the Pages
// route table — it is imported, never served.

// csvCell quotes one value per RFC 4180 and defuses spreadsheet formulas.
// Every column but the timestamps started as visitor input, and a cell that
// begins with = + - @ (or a tab / carriage return) is executed as a formula by
// Excel, LibreOffice and Google Sheets when the export is opened. A leading
// apostrophe makes the spreadsheet show it as text instead.
export function csvCell(value: unknown): string {
  let s = value == null ? "" : String(value);
  if (/^[=+\-@\t\r]/.test(s)) s = "'" + s;
  return /[",\r\n]/.test(s) || s !== s.trim() ? `"${s.replaceAll('"', '""')}"` : s;
}

// toCSV renders a header row plus one line per record, CRLF-terminated as
// RFC 4180 specifies, with a UTF-8 byte-order mark so Excel does not read
// Polish or Hindi text as mojibake.
export function toCSV(columns: string[], rows: Record<string, unknown>[]): string {
  const lines = [columns.map(csvCell).join(",")];
  for (const row of rows) lines.push(columns.map((c) => csvCell(row[c])).join(","));
  return "﻿" + lines.join("\r\n") + "\r\n";
}
