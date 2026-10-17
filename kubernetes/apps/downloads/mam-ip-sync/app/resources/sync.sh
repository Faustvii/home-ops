#!/bin/sh
# Registers the VPN egress IP with MyAnonamouse's dynamic seedbox endpoint.
# Every request goes through vpn-proxy, so MAM sees the same VPN IP as qbooks.
set -eu

CACHE=/data/MAM.ip
BASE=https://t.myanonamouse.net/json

# Public IPv4 address through the VPN (ipify returns plain text).
NEWIP=$(curl -fsS --max-time 30 -x "$PROXY" https://api.ipify.org)
if [ -z "$NEWIP" ]; then
  echo "Could not determine the VPN IP" >&2
  exit 1
fi

OLDIP=$(cat "$CACHE" 2>/dev/null || true)
if [ "$OLDIP" = "$NEWIP" ]; then
  echo "Nothing to do - $NEWIP"
  exit 0
fi

echo "New IP detected: old=$OLDIP new=$NEWIP"
RESPONSE=$(curl -sS --max-time 30 -x "$PROXY" -b "mam_id=$MAM_ID" "$BASE/dynamicSeedbox.php")
echo "$RESPONSE"

if echo "$RESPONSE" | grep -q '"Success":true'; then
  echo "$NEWIP" > "$CACHE"
  echo "New IP cached"
else
  MSG=$(echo "$RESPONSE" | sed -n 's/.*"msg":"\([^"]*\)".*/\1/p')
  [ -n "$MSG" ] || MSG="Unknown error"
  # MAM allows one change per hour; the cache stays stale, so the next run retries
  if [ "$MSG" = "Last change too recent" ]; then
    echo "Rate limited by MAM, will retry"
    exit 0
  fi
  echo "Failed to update - Error: $MSG" >&2
  exit 1
fi
