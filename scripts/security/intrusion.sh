#!/bin/bash
# Intrusion tests against the public surface, run from outside it.
#
# The vantage point is the point. Everything here is what an attacker with the
# hostname and nothing else can reach: no SSH, no localhost, no knowledge of the
# internals. A check that passes only because it was run on the box is not a
# check.
#
# Each test prints PASS or FAIL and what it means. FAIL is a finding, not a
# crash.
set -uo pipefail

H=yamale.ddns.net
IP=92.4.151.72
pass=0; fail=0
ok()   { echo "  PASS  $1"; pass=$((pass+1)); }
bad()  { echo "  FAIL  $1"; fail=$((fail+1)); }

echo "=== 1. consensus and node ports ==="
# CometBFT RPC on 26657 is a remote control: it broadcasts transactions, dumps
# consensus state, and on older builds exposes unsafe endpoints. It must never
# face the internet.
for port in 26657 26656 9090 1317; do
  if timeout 6 bash -c "echo > /dev/tcp/$IP/$port" 2>/dev/null; then
    bad "port $port is open to the internet"
  else
    ok "port $port closed"
  fi
done

echo
echo "=== 2. RPC reachable through the web path? ==="
for p in /rpc/status /api/rpc/status /26657/status /rpc /status; do
  code=$(curl -s -o /dev/null -w "%{http_code}" --max-time 15 "https://$H$p")
  [ "$code" = "200" ] && bad "$p answered 200 (RPC exposed via HTTP)" || ok "$p -> $code"
done

echo
echo "=== 3. transaction broadcast from outside ==="
# If an attacker can reach any broadcast path they can spend other people's gas
# grants and spam the mempool.
for p in /api/rest/cosmos/tx/v1beta1/txs /api/tx /broadcast_tx_sync; do
  code=$(curl -s -o /dev/null -w "%{http_code}" --max-time 15 -X POST \
         -H 'Content-Type: application/json' -d '{"tx_bytes":"","mode":"BROADCAST_MODE_SYNC"}' \
         "https://$H$p")
  # 400/401/404 are all fine; 200 would mean it accepted an unauthenticated broadcast.
  [ "$code" = "200" ] && bad "$p accepted an unauthenticated broadcast" || ok "$p -> $code"
done

echo
echo "=== 4. the supervisor gate ==="
gated=(
  "cosmos/auth/v1beta1/accounts"
  "cosmos/bank/v1beta1/supply"
  "cosmos/base/tendermint/v1beta1/node_info"
)
for g in "${gated[@]}"; do
  code=$(curl -s -o /dev/null -w "%{http_code}" --max-time 15 "https://$H/api/rest/$g")
  case "$code" in
    401|403) ok "$g gated ($code)" ;;
    200)     bad "$g is public ($code)" ;;
    *)       ok "$g -> $code" ;;
  esac
done

echo
echo "=== 5. credential guessing against the gate ==="
for creds in admin:admin yamale:yamale supervisor:supervisor admin:password root:root; do
  code=$(curl -s -o /dev/null -w "%{http_code}" --max-time 15 -u "$creds" \
         "https://$H/api/rest/cosmos/bank/v1beta1/supply")
  [ "$code" = "200" ] && bad "credentials $creds ACCEPTED" || true
done
ok "no default credential pair accepted"

echo
echo "=== 6. path traversal and file disclosure ==="
for p in "/../../etc/passwd" "/app/../../../etc/passwd" "/%2e%2e%2f%2e%2e%2fetc%2fpasswd" \
         "/.env" "/.git/config" "/config.toml" "/priv_validator_key.json" \
         "/app/.env" "/explorer/.git/config" "/node_key.json"; do
  # A SPA fallback (try_files ... /index.html) answers 200 with index.html for
  # any path at all, so a status code alone reads every miss as a disclosure.
  # What matters is whether the body is the app shell or the actual file.
  code=$(curl -s -o /dev/null -w "%{http_code}" --max-time 15 "https://$H$p")
  html=$(curl -s --max-time 15 "https://$H$p" | head -c 200 | grep -ci "<!doctype html\|<html")
  if [ "$code" = "200" ] && [ "$html" = "0" ]; then bad "$p served real content"
  else ok "$p -> $code"; fi
done

echo
echo "=== 7. the validator key, by any name ==="
# The one file whose disclosure ends the chain.
for p in /priv_validator_key.json /config/priv_validator_key.json /node/priv_validator_key.json \
         /api/priv_validator_key.json /keyring-test /node/keyring-test; do
  code=$(curl -s -o /dev/null -w "%{http_code}" --max-time 12 "https://$H$p")
  [ "$code" = "200" ] && bad "$p SERVED" || ok "$p -> $code"
done

echo
echo "=== 8. transport security ==="
tls=$(curl -s -o /dev/null -w "%{http_version} %{ssl_verify_result}" --max-time 15 "https://$H/")
echo "  http/tls: $tls"
redirect=$(curl -s -o /dev/null -w "%{http_code}" --max-time 15 "http://$H/")
[ "$redirect" = "301" ] || [ "$redirect" = "308" ] && ok "plain HTTP redirects ($redirect)" || bad "plain HTTP -> $redirect (no redirect)"
hsts=$(curl -sI --max-time 15 "https://$H/" | grep -ci "strict-transport-security")
[ "$hsts" -gt 0 ] && ok "HSTS present" || bad "no HSTS header"

echo
echo "=== 9. response headers ==="
hdrs=$(curl -sI --max-time 15 "https://$H/app/")
for h in x-frame-options x-content-type-options content-security-policy referrer-policy; do
  echo "$hdrs" | grep -qi "^$h" && ok "$h set" || bad "$h missing"
done
echo "$hdrs" | grep -qiE "^server: nginx/[0-9]" && bad "server version disclosed: $(echo "$hdrs" | grep -i '^server:' | tr -d '\r')" || ok "server version not disclosed"

echo
echo "=== 10. faucet abuse ==="
# The faucet spends real balance. Its limits are the only thing between a demo
# and a drained foundation account.
victim=yml1u0hvh9cg8jfjg9n8llz2g0jn7zjwa8deaacwu3
granted=0
for i in 1 2 3 4 5; do
  r=$(curl -s --max-time 40 -X POST "https://$H/api/faucet/" \
      -H 'Content-Type: application/json' -d "{\"address\":\"$victim\"}")
  echo "$r" | grep -q '"sent"' && granted=$((granted+1))
done
[ "$granted" -le 1 ] && ok "faucet rate-limited (granted $granted of 5)" \
                     || bad "faucet granted $granted of 5 rapid requests"

echo
echo "=== 11. request flooding ==="
codes=$(for i in $(seq 1 60); do
  curl -s -o /dev/null -w "%{http_code}\n" --max-time 10 \
    "https://$H/api/rest/cosmos/bank/v1beta1/denoms_metadata?pagination.limit=1" &
done | sort | uniq -c | tr '\n' ' ')
wait
echo "  60 concurrent: $codes"
echo "$codes" | grep -qE "429|503" && ok "rate limiting engaged under flood" || bad "no rate limiting observed under 60 concurrent requests"

echo
echo "=== 12. host header and cache poisoning ==="
code=$(curl -s -o /dev/null -w "%{http_code}" --max-time 15 -H "Host: evil.example" "https://$H/")
echo "  spoofed Host -> $code"
loc=$(curl -sI --max-time 15 -H "X-Forwarded-Host: evil.example" "https://$H/" | grep -i "^location" | tr -d '\r')
[ -n "$loc" ] && echo "  X-Forwarded-Host reflected in: $loc" || ok "X-Forwarded-Host not reflected"

echo
echo "=== 13. CORS ==="
acao=$(curl -sI --max-time 15 -H "Origin: https://evil.example" "https://$H/api/rest/cosmos/bank/v1beta1/denoms_metadata?pagination.limit=1" | grep -i "access-control-allow-origin" | tr -d '\r')
if echo "$acao" | grep -q "\*"; then
  echo "  NOTE  $acao — wildcard on read-only public data, acceptable but worth knowing"
elif [ -n "$acao" ]; then
  echo "  $acao"
else
  ok "no CORS header (same-origin only)"
fi

echo
echo "======================================"
echo "  passed: $pass    findings: $fail"
echo "======================================"
