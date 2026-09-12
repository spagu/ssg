#!/usr/bin/env sh
# Fills the local shop with the two books in content/, so the storefront has
# something to sell a minute after `ssg --watch` starts.
#
#   sh examples/ebook-shop/scripts/seed.sh
#
# It talks to the running `wrangler pages dev` over HTTP — the same API the
# panel uses — rather than writing to the database behind the worker's back.
# Everything it does can be undone in the panel.
set -eu

BASE="${SHOP_BASE:-http://localhost:8788}"
EMAIL="${SHOP_EMAIL:-owner@example.com}"
PASSWORD="${SHOP_PASSWORD:-correct-horse-battery-staple}"
HERE=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)

say() { printf '%s\n' "$*"; }
json() { python3 -c "import json,sys;d=json.load(sys.stdin);print(d$1)"; }

say "→ signing in at $BASE"
TOKEN=$(curl -fsS -X POST "$BASE/api/shop/admin/auth/login" \
  -H 'content-type: application/json' \
  -d "{\"email\":\"$EMAIL\",\"password\":\"$PASSWORD\"}" | json "['accessToken']")
AUTH="authorization: Bearer $TOKEN"

say "→ settings"
curl -fsS -o /dev/null -X PUT "$BASE/api/shop/admin/settings" -H "$AUTH" \
  -H 'content-type: application/json' -d '{
    "seller.name": "Paperless Press sp. z o.o.",
    "seller.address": "ul. Przykladowa 1\n00-001 Warszawa",
    "seller.country": "PL",
    "seller.vat_id": "PL0000000000",
    "seller.email": "help@example.com",
    "shop.name": "Paperless Press",
    "currency.base": "EUR",
    "pricing.mode": "gross",
    "tax.mode": "table"
  }'

# sku | name | price in cents | file | cover
BOOKS='EBOOK-SHOP|The One-Person Shop|2400|the-one-person-shop.pdf|/images/cover-one-person-shop.svg
EBOOK-10K|Ten Thousand Words a Month|1900|ten-thousand-words-a-month.pdf|/images/cover-ten-thousand-words.svg'

printf '%s\n' "$BOOKS" | while IFS='|' read -r SKU NAME PRICE FILE COVER; do
  say "→ $SKU"
  # Creating a product that already exists answers 409; the id is then looked
  # up instead, so running this script twice is harmless.
  CREATED=$(curl -sS -X POST "$BASE/api/shop/admin/products" -H "$AUTH" \
    -H 'content-type: application/json' \
    -d "{\"sku\":\"$SKU\",\"name\":\"$NAME\"}" || true)
  ID=$(printf '%s' "$CREATED" | python3 -c "
import json,sys
try:
    print(json.load(sys.stdin)['product']['id'])
except Exception:
    print('')
")
  if [ -z "$ID" ]; then
    ID=$(curl -fsS "$BASE/api/shop/admin/products" -H "$AUTH" | python3 -c "
import json,sys
for p in json.load(sys.stdin)['products']:
    if p['sku'] == '$SKU':
        print(p['id'])
        break
")
  fi

  curl -fsS -o /dev/null -X POST "$BASE/api/shop/admin/products/$ID/file" \
    -H "$AUTH" -F "file=@$HERE/files/$FILE"

  curl -fsS -o /dev/null -X PATCH "$BASE/api/shop/admin/products/$ID" -H "$AUTH" \
    -H 'content-type: application/json' \
    -d "{\"prices\":{\"EUR\":$PRICE},\"status\":\"active\",\"taxCategory\":\"ebook\",\"imageUrl\":\"$COVER\"}"
done

say ""
say "Done. The shop now sells two books:"
curl -fsS "$BASE/api/shop/products" | python3 -c "
import json,sys
for p in json.load(sys.stdin)['products']:
    price = next(iter(p['prices'].items()), ('EUR', 0))
    print('  %-12s %-32s %6.2f %s' % (p['sku'], p['name'], price[1] / 100, price[0]))
"
say ""
say "Storefront: $BASE"
say "Panel:      $BASE/ecommerce-admin/  ($EMAIL)"
