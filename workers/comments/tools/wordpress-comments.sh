#!/usr/bin/env bash
#
# Pull comments from a WordPress site's REST API and convert them to the SSG
# comments worker's import JSON. No plugin, no export file — just the public
# REST API of the old site. Requires: curl, jq.
#
#   ./wordpress-comments.sh https://old-blog.example.com > comments.json
#
#   curl -u :"$COMMENTS_ADMIN_PASSWORD" -H 'content-type: application/json' \
#        --data @comments.json https://your-site/api/comments/import
#
# Or fetch, convert AND post in one go (chunked under the 1000/request cap):
#
#   ./wordpress-comments.sh https://old-blog.example.com \
#       --post https://your-site/api/comments/import --password "$COMMENTS_ADMIN_PASSWORD"
#
# Each comment's URL is built from the post slug via URL_TEMPLATE (default
# "/blog/{slug}/") — set it to match your permalink pattern:
#
#   URL_TEMPLATE='/{slug}/' ./wordpress-comments.sh https://old-blog.example.com
#
# Notes: the public REST API returns APPROVED comments only, and does not expose
# commenter emails (so imported comments have no Gravatar). For pending/spam or
# emails, export the WXR file instead (Tools -> Export) — see the worker README.
# The import is idempotent, so re-running is safe.
set -euo pipefail

die() { echo "error: $*" >&2; exit 1; }

SRC="${1:-}"
[ -n "$SRC" ] || die "usage: wordpress-comments.sh https://OLD-WORDPRESS [--post URL --password PASS]"
SRC="${SRC%/}"
shift || true

POST_URL=""
PASSWORD=""
while [ $# -gt 0 ]; do
  case "$1" in
    --post) POST_URL="${2:?}"; shift 2 ;;
    --password) PASSWORD="${2:?}"; shift 2 ;;
    *) die "unknown argument: $1" ;;
  esac
done
URL_TEMPLATE="${URL_TEMPLATE:-/blog/{slug}/}"

command -v jq >/dev/null || die "jq is required"
command -v curl >/dev/null || die "curl is required"

# fetch_all <endpoint-with-query> — follows X-WP-TotalPages pagination and emits
# each page's JSON array on its own line.
fetch_all() {
  local path="$1" page=1 pages hdr
  hdr="$(mktemp)"
  while :; do
    curl -fsS -D "$hdr" "${SRC}/wp-json/wp/v2/${path}&page=${page}" || die "fetch failed: ${path} (page ${page})"
    pages="$(awk -F': ' 'tolower($1)=="x-wp-totalpages"{gsub(/[\r ]/,"",$2);print $2}' "$hdr")"
    [ -n "$pages" ] || pages=1
    [ "$page" -ge "$pages" ] && break
    page=$((page + 1))
  done
  rm -f "$hdr"
}

echo "fetching posts (id -> slug) from ${SRC}…" >&2
posts="$(fetch_all 'posts?per_page=100&status=publish&_fields=id,slug' | jq -s 'add // []')"
echo "fetching approved comments…" >&2
comments="$(fetch_all 'comments?per_page=100&_fields=post,author_name,content,date_gmt' | jq -s 'add // []')"

# Join comment.post -> slug and map to the import shape. Comment content is HTML
# (the widget renders text), so strip tags and unescape the common entities.
items="$(jq -n \
  --argjson posts "$posts" \
  --argjson comments "$comments" \
  --arg tmpl "$URL_TEMPLATE" '
  ($posts | map({ (.id|tostring): .slug }) | add // {}) as $slug
  | def clean:
      gsub("<[^>]+>"; "")
      | gsub("&amp;";"&") | gsub("&lt;";"<") | gsub("&gt;";">")
      | gsub("&quot;";"\"") | gsub("&#0?39;|&#8217;|&#8216;";"\u0027")
      | gsub("&#8220;|&#8221;";"\"") | gsub("&nbsp;";" ") | gsub("&hellip;";"…")
      | gsub("^\\s+|\\s+$";"");
  $comments
  | map(select($slug[.post|tostring] != null))
  | map(
      ($slug[.post|tostring]) as $s
      | {
          url:    ($tmpl | gsub("\\{slug\\}"; $s)),
          author: ((.author_name // "")[0:80]),
          body:   ((.content.rendered // "") | clean | .[0:5000]),
          status: "approved",
          created_at: ((.date_gmt // "") | if . == "" then empty else . + "Z" end)
        })
  | map(select(.author != "" and .body != ""))
')"

count="$(jq 'length' <<<"$items")"
echo "converted ${count} comments" >&2

if [ -z "$POST_URL" ]; then
  printf '%s\n' "$items"
  exit 0
fi

[ -n "$PASSWORD" ] || die "--post requires --password (COMMENTS_ADMIN_PASSWORD)"
auth="$(printf ':%s' "$PASSWORD" | base64)"
n=0
while [ "$n" -lt "$count" ]; do
  chunk="$(jq -c --argjson n "$n" '{items: .[$n:($n+500)]}' <<<"$items")"
  echo "posting $((n + 1))..$((n < count - 500 ? n + 500 : count))" >&2
  curl -fsS -X POST "$POST_URL" \
    -H 'content-type: application/json' \
    -H "authorization: Basic ${auth}" \
    --data "$chunk" >&2
  echo >&2
  n=$((n + 500))
done
