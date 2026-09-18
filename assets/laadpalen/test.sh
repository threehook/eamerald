#!/usr/bin/env bash
# Runs every case in test_cases.json against a live eamerald authorizer's
# request_laadpaal decision and reports pass/fail per case.
#
# Usage: assets/laadpalen/test.sh [authorizer-url]
# (defaults to https://localhost:8383)
set -euo pipefail

AUTHORIZER_URL="${1:-https://localhost:8383}"
CASES_FILE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/test_cases.json"

pass=0
fail=0

count=$(jq 'length' "$CASES_FILE")

for i in $(seq 0 $((count - 1))); do
  tc=$(jq -c ".[$i]" "$CASES_FILE")
  user=$(jq -r '.user' <<<"$tc")
  postcode=$(jq -r '.postcode' <<<"$tc")
  huisnummer=$(jq -r '.huisnummer' <<<"$tc")
  want_decision=$(jq -r '.decision' <<<"$tc")
  want_reason=$(jq -r '.reason' <<<"$tc")

  body=$(jq -n \
    --arg identity "$user" \
    --arg postcode "$postcode" \
    --argjson huisnummer "$huisnummer" \
    '{
      query: "x = data.authz.request_laadpaal",
      identity_context: {type: "IDENTITY_TYPE_SUB", identity: $identity},
      resource_context: {postcode: $postcode, huisnummer: $huisnummer}
    }')

  response=$(curl -sk -X POST "$AUTHORIZER_URL/api/v2/authz/query" \
    -H "Content-Type: application/json" \
    -d "$body")

  got_decision=$(jq -r '.response.result[0].bindings.x.decision' <<<"$response")
  got_reason=$(jq -r '.response.result[0].bindings.x.context.reason' <<<"$response")

  label="$user $postcode/$huisnummer"

  if [ "$got_decision" = "$want_decision" ] && [ "$got_reason" = "$want_reason" ]; then
    printf "PASS  %-30s -> %s\n" "$label" "$got_reason"
    pass=$((pass + 1))
  else
    printf "FAIL  %-30s -> got: decision=%s reason=%s\n" "$label" "$got_decision" "$got_reason"
    printf "      %-30s    want: decision=%s reason=%s\n" "" "$want_decision" "$want_reason"
    fail=$((fail + 1))
  fi
done

echo
echo "$pass/$((pass + fail)) passed"

[ "$fail" -eq 0 ]
