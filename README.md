# artemis-handbrake

**Artemis Handbrake** — Kubernetes operator (Go) that watches a Deployment’s pod memory vs limits and pauses/resumes an Apache Artemis (AMQ Broker) **address** via Jolokia when thresholds are crossed.

**Module:** `github.com/gmodzelewski/artemis-handbrake`

| Doc | Purpose |
|-----|---------|
| [docs/DESIGN.plan.md](./docs/DESIGN.plan.md) | Full design (Prometheus vs operator, hysteresis, naming) |
| [docs/OPERATOR-SOURCE.md](./docs/OPERATOR-SOURCE.md) | Go file tree + paste-ready code until `*.go` live in this repo |
| [DEPLOYMENT.md](./DEPLOYMENT.md) | OpenShift: build, RBAC, CRD, operator, CR, networking |

Canonical folder for this project: **`gmodzelewski/artemis-handbrake`** (this directory).

## Webhook receiver (implemented)

- **Source:** `cmd/handbrake-webhook`, `pkg/jolokia`
- **OpenShift test stack:** `deploy/test/` (`oc apply -k deploy/test`, then `oc start-build` as in [deploy/test/README.md](deploy/test/README.md))

Smoke test (synthetic Alertmanager payload):

```bash
oc run smoke --image=curlimages/curl:8.5.0 --restart=Never -n artemis-handbrake-test --command -- \
  curl -sS -X POST http://handbrake-webhook:8080/webhook \
  -H 'Content-Type: application/json' \
  -d '{"status":"firing","alerts":[{"status":"firing","labels":{"alertname":"WorkloadMemoryHigh"}}]}'
```
