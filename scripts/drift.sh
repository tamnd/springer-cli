#!/usr/bin/env bash
#
# Probe the live site with the urls whose behaviour the classifier is built on,
# and report when one of them stops behaving the way it was measured.
#
# Every line below is a fact this tool depends on being true. An open access
# article is readable, a chapter behind a subscription answers 200 with
# access=No, a pdf url without a subscription answers html after a seven hop
# detour, a work that does not exist answers 404 with a large body, and the rss
# search feed answers at all. If any of those changes, something upstream in the
# code is now wrong, and the point of this script is that we hear about it from
# a scheduled run rather than from a person filing a bug.
#
# It is deliberately small. Six requests at the default pace is a rounding error
# on Springer's bandwidth, and a weekly job that costs nothing is a weekly job
# nobody is tempted to switch off.

set -uo pipefail

SPR="${SPR:-./bin/spr}"
fail=0

# probe <expected status> <kind> <url> <what this proves>
probe() {
  local want="$1" kind="$2" url="$3" why="$4"
  local out got

  out=$("$SPR" get --no-cache --kind "$kind" -o json "$url" 2>&1)
  got=$(printf '%s' "$out" | sed -n 's/.*"status": "\([a-z_]*\)".*/\1/p' | head -1)

  if [ "$got" = "$want" ]; then
    printf 'ok       %-11s %s\n' "$got" "$url"
    return 0
  fi

  fail=1
  printf 'CHANGED  want %s, got %s\n         %s\n         %s\n' \
    "$want" "${got:-no answer}" "$url" "$why"
  printf '%s\n' "$out" | sed 's/^/         /'
}

echo "spr drift check, $(date -u '+%Y-%m-%d %H:%M UTC')"
echo

probe ok html \
  "/article/10.1007/s10994-021-05946-3" \
  "An open access article should be readable in full."

probe restricted html \
  "/chapter/10.1007/978-3-031-28170-9_5" \
  "A chapter behind a subscription should answer 200 with access=No, which is how Restricted is told apart from OK."

probe ok pdf \
  "/content/pdf/10.1007/s10994-021-05946-3.pdf" \
  "An open access pdf url should answer with an actual pdf."

probe wrong_kind pdf \
  "/content/pdf/10.1007/978-3-031-28170-9_5.pdf" \
  "A pdf url without a subscription should redirect across to the chapter page and answer html, which is why a 200 is not enough to call it a pdf."

probe not_found html \
  "/article/10.1007/this-work-does-not-exist" \
  "A missing work should answer 404, with a body large enough that nothing can be inferred from size."

probe ok xml \
  "/search.rss?query=uncertainty" \
  "The rss search feed should answer, because it is the search path that is not challenged."

echo
if [ "$fail" -eq 0 ]; then
  echo "Every probe matched what was measured."
else
  echo "At least one probe changed. The site moved and the code has not."
fi
exit "$fail"
