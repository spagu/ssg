# Pronunciation sets

What the narrator says instead of what is written: "250g" → "250 gramów",
"1 łyżeczka" → "jedna łyżeczka", "180°C" → "180 stopni Celsjusza".
Used by the browser narrator (`story-say.js`) and the recorded Web Story
narration (`story_say.go`); the text on screen and in subtitles is never changed.

One file per language, named like the page's `lang`: `pl_PL.json` serves
`pl-PL` and, when no closer file exists, any `pl-*` page.

```json
{ "lang": "pl_PL", "about": "…", "rules": { "pattern": "said" }, "examples": [["written", "said"]] }
```

`examples` are tests: every one must read as written down, in both engines
(`make test-story` runs the JavaScript side, `go test ./internal/generator` the Go side).

## Syntax

| In a pattern | Means |
|---|---|
| `{num}` | a number: `100`, `1,5`, `0.25` |
| a space | a space or none: `"{num} g"` matches `100g` and `100 g` |
| `a\|b\|c` | any of these words |

Patterns match whole words only (`"{num} l"` leaves `5 lat` alone); the longest pattern wins.

| In what is said | Means |
|---|---|
| `{num}` | the number as written (voices read digits well) |
| `{num:jedna\|dwie}` | the number in words, with 1 and a final 2 in this gender: 1 → jedna, 22 → dwadzieścia dwie (Polish only; fractions stay digits) |
| `{word}` | the word the pattern's `a\|b\|c` matched, as written (keeps its case) |
| `a\|b\|c\|d` | the form agreeing with the number before it, in CLDR plural order — Polish: 1 \| 2–4 \| 5+ \| fractions; English: 1 \| other |

## On a site

```yaml
story:
  narrator:
    pronounce:                       # added over the built-in set
      "{num} blaszka|blaszki|blaszek": "{num:jedna|dwie} {word}"
      "np.": null                    # turn one built-in rule off
    # pronounceDefaults: false       # no built-in set at all
```

A post's own story file can add or override rules the same way.
