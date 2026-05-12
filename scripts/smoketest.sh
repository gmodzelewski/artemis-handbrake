#!/usr/bin/env bash
set -euo pipefail
NS="${1:-artemis-handbrake-test}"
POD="handbrake-smoke-$(date +%s)"
oc run "$POD" --image=curlimages/curl:8.5.0 --restart=Never -n "$NS" --command -- \
  curl -sS -f -X POST "http://handbrake-webhook.${NS}.svc.cluster.local:8080/webhook" \
  -H 'Content-Type: application/json' \
  -d '{"status":"firing","alerts":[{"status":"firing","labels":{"alertname":"WorkloadMemoryHigh"}}]}'
oc wait --for=condition=Ready "pod/$POD" -n "$NS" --timeout=60s >/dev/null
oc logs -n "$NS" "pod/$POD"
oc delete pod "$POD" -n "$NS" --wait=false
