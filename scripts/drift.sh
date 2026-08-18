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
# It is deliberately small. Seven requests at the default pace is a rounding
# error on Springer's bandwidth, and a weekly job that costs nothing is a weekly
# job nobody is tempted to switch off.
#
# A changed probe is news and not a build failure. This script exits nonzero so
# that a person running it locally gets an exit code, and the workflow reads
# that code and opens an issue with it rather than going red, because a red
# weekly job trains everybody to ignore the weekly job.

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

# challenge <url> <what this proves>
#
# The one probe with two right answers. The html search surface is the surface
# Fastly puts its client challenge in front of, and whether it does that on any
# given day depends on where the request came from, so a run from a datacenter
# is challenged and a run from a laptop usually is not. Both are fine and which
# one happened is the news.
#
# What is not fine is a 200 the size of the interstitial that this tool called a
# served page, which is what it looks like the day the wording in
# challengeMarker changes. The size test is the whole reason this probe exists:
# the challenge is 3,038 bytes and a real search result page is 380 KB, so
# nothing subtle is being inferred here.
challenge() {
  local url="$1" why="$2"
  local out got bytes

  out=$("$SPR" get --no-cache --kind html -o json "$url" 2>&1)
  got=$(printf '%s' "$out" | sed -n 's/.*"status": "\([a-z_]*\)".*/\1/p' | head -1)
  bytes=$(printf '%s' "$out" | sed -n 's/.*"bytes": \([0-9]*\).*/\1/p' | head -1)

  case "$got" in
  challenged)
    printf 'ok       %-11s %s\n         the interstitial still says Client Challenge, so the marker is current\n' \
      "$got" "$url"
    return 0
    ;;
  ok)
    if [ "${bytes:-0}" -ge 100000 ]; then
      printf 'ok       %-11s %s\n         a real page of %s bytes, so this run was not challenged\n' \
        "$got" "$url" "$bytes"
      return 0
    fi
    fail=1
    printf 'CHANGED  ok in %s bytes, which is interstitial sized\n         %s\n         %s\n' \
      "${bytes:-0}" "$url" "$why"
    return 0
    ;;
  esac

  fail=1
  printf 'CHANGED  want ok or challenged, got %s\n         %s\n         %s\n' \
    "${got:-no answer}" "$url" "$why"
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

challenge \
  "/search?query=uncertainty" \
  "Either the challenge is being served and not recognised, or the search surface now answers something small that is not the challenge. Read the body before changing challengeMarker."

echo
if [ "$fail" -eq 0 ]; then
  echo "Every probe matched what was measured."
else
  echo "At least one probe changed. The site moved and the code has not."
fi
exit "$fail"
