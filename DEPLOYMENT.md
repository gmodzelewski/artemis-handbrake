# OpenShift deployment plan — artemis-handbrake

This document orders everything you need to run **Artemis Handbrake** on a connected OpenShift cluster. Adjust names (`NAMESPACE`, `IMAGE`, `Deployment` targets) to match your environment.

## Prerequisites

1. **`oc` logged in** with rights to create CRDs (cluster admin or CRD apply pre-done by platform), Deployments, Roles, RoleBindings, and Secrets in your target namespaces.
2. **Metrics Server path available** to the operator: the reconciler uses **`metrics.k8s.io/v1beta1` PodMetrics**. On OpenShift this is normally provided by the monitoring stack; if pod metrics are empty, fix monitoring before relying on the brake.
3. **AMQ Broker / Artemis** reachable from the operator pod: **Jolokia** URL (often `https://<broker-svc>:8161/console/jolokia` or operator-exposed route). Know **`broker-name`** (JMX segment) and **address** name.
4. **Jolokia admin credentials** (or a dedicated user allowed to `pause` / `resume` the address).

## 0. Repository layout (this folder)

Place generated Go sources beside this file (`main.go`, `api/`, `controllers/`, `pkg/`, `config/`). See [docs/OPERATOR-SOURCE.md](./docs/OPERATOR-SOURCE.md). Replace module path with:

`github.com/gmodzelewski/artemis-handbrake`

Build a container image and push to a registry OpenShift can pull (e.g. **Quay.io** `quay.io/gmodzelewski/artemis-handbrake:latest` or **OpenShift internal registry**).

## 1. Namespaces (recommended split)

| Namespace | Holds |
|-----------|--------|
| `artemis-handbrake-system` | Operator Deployment, SA, RBAC for the operator, `BrokerMemoryBrake` CRs (recommended) |
| `my-amq` (example) | AMQ Broker, workloads you measure, Jolokia Secret copy or reference |

You may run the operator in **`my-amq`** instead if you prefer a single namespace; then tighten RBAC to **Role** in that namespace only.

## 2. Install CRD (cluster scope)

Apply once per cluster (needs CRD create permission):

```bash
oc apply -f config/crd/bases/artemisbrake.amq.io_brokermemorybrakes.yaml
```

Verify:

```bash
oc get crd brokermemorybrakes.artemisbrake.amq.io
```

## 3. ServiceAccount and RBAC

**Option A — operator in `artemis-handbrake-system`, watches workloads in `my-amq`**

- **ClusterRole** (or two **Role**s + **RoleBinding**s) must allow at minimum:
  - `artemisbrake.amq.io` `brokermemorybrakes` get, list, watch, update, patch; `brokermemorybrakes/status` get, update, patch (namespace: `artemis-handbrake-system`).
  - `apps` `deployments` get, list, watch in **`my-amq`** (and any other watched namespace).
  - `""` `pods`, `secrets` get, list, watch in **`my-amq`** (secrets only if you later read broker creds from there; default sample reads Secret next to CR in operator namespace).
  - `metrics.k8s.io` `pods` get, list in **`my-amq`** (PodMetrics per pod).

Bind the **ClusterRole** to the operator **ServiceAccount** with a **ClusterRoleBinding**, or use **Aggregated** roles if your org prefers.

**Option B — single namespace**

- **Role** + **RoleBinding** in `my-amq` only; operator and CR and broker colocated; simplest RBAC.

Example SA + RoleBinding (names are illustrative):

```yaml
# config/openshift/00-sa.yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: artemis-handbrake-controller-manager
  namespace: artemis-handbrake-system
```

Apply `config/rbac/role.yaml` / `role_binding.yaml` from the repo, **after** expanding rules for every namespace the CR’s `spec.targetNamespace` may reference (or use a generated ClusterRole from `controller-gen rbac`).

## 4. Jolokia credentials Secret

In the **same namespace as the `BrokerMemoryBrake` CR** (per sample):

```bash
oc create secret generic jolokia-admin \
  --from-literal=username='...' \
  --from-literal=password='...' \
  -n artemis-handbrake-system
```

Use the same credentials you use for the Artemis console / Jolokia.

## 5. Build and push the operator image

**External registry (typical):**

```bash
docker build -t quay.io/gmodzelewski/artemis-handbrake:latest .
docker push quay.io/gmodzelewski/artemis-handbrake:latest
```

Create **pull secret** on OpenShift if the image is private:

```bash
oc create secret docker-registry artemis-handbrake-pull-secret \
  --docker-server=quay.io --docker-username=... --docker-password=... \
  -n artemis-handbrake-system
oc secrets link serviceaccount/artemis-handbrake-controller-manager pull-secret artemis-handbrake-pull-secret --for=pull -n artemis-handbrake-system
```

**OpenShift build (in-cluster):**

```bash
oc new-build --name=artemis-handbrake --binary --strategy=docker -n artemis-handbrake-system
# from repo root, after Dockerfile exists:
oc start-build artemis-handbrake --from-dir=. --follow -n artemis-handbrake-system
```

Use the resulting `ImageStreamTag` in the Deployment `image:` field.

## 6. Operator Deployment

Apply `config/manager/manager.yaml` (create if not present) with:

- `serviceAccountName: artemis-handbrake-controller-manager`
- `image: quay.io/gmodzelewski/artemis-handbrake:latest` (or internal registry URL)
- **Requests/limits** small (CPU 100m, memory 128Mi class).
- **Args:** default metrics `:8080`, probes `:8081` (match Service/Route if you expose metrics).
- **`WATCH_NAMESPACE`:** not required for a cluster-scoped manager using controller-runtime defaults (watches all namespaces for CRs); if you inject a single-namespace mode later, set env accordingly.

**Probes:** `GET :8081/healthz` and `readyz` (from `main.go` snippet).

**SCC:** Use restricted / restricted-v2; run as non-root (distroless image).

## 7. NetworkPolicy (recommended)

- **Ingress:** none required unless you scrape operator metrics from outside the namespace.
- **Egress allow:**
  - TCP **443** to **Kubernetes API** (openshift.default.svc or API VIP).
  - TCP **443** or **8161** to **AMQ Broker Service** host for Jolokia (match your `jolokiaBaseURL`).
  - DNS to cluster DNS.

If NetworkPolicies default-deny egress, add explicit rules; otherwise document “no NP” risk.

## 8. TLS to Jolokia

If the broker presents a **private CA** or **service cert**, either:

- Add the broker CA to a **ConfigMap** and mount trust into the operator image (requires rebuilding base or using a wrapper image with `update-ca-trust`), or  
- For lab only: configure HTTP client trust (not recommended for production).

Prefer **cluster-internal Service** DNS and CA bundle that already trusts the broker issuer.

## 9. Apply `BrokerMemoryBrake` CR

Edit [config/samples](./config/samples) (from OPERATOR-SOURCE) so that:

- `spec.targetDeployment` / `spec.targetNamespace` point at the **consumer** (or any) Deployment whose memory you measure.
- `spec.jolokiaBaseURL` / `spec.jolokiaOrigin` match your broker console/Jolokia URL.
- `spec.brokerName` / `spec.artemisAddress` match JMX reality.
- Thresholds and `pollIntervalSeconds` are set.

```bash
oc apply -f config/samples/artemisbrake_v1alpha1_brokermemorybrake.yaml -n artemis-handbrake-system
```

## 10. Verification

```bash
oc get brokermemorybrake -A
oc describe brokermemorybrake consumer-brake -n artemis-handbrake-system
oc logs deploy/artemis-handbrake-controller-manager -n artemis-handbrake-system -f
```

Confirm `status.lastMaxRatioPercent` moves with load and `status.lastPhase` shows `paused` / `resumed` / `idle` as expected. On the broker, confirm address pause via console or Jolokia `isPaused` read.

## 11. Day-2

- **Upgrade:** roll image tag; CRD version bumps need migration notes.
- **Multiple policies:** one CR per (target deployment, address) pair.
- **Alerting:** optional `PrometheusRule` on operator errors (reconcile failures) if you add metrics.

## Checklist summary

1. Push **image** OpenShift can pull.  
2. **`oc apply` CRD**.  
3. **Namespace(s)** + **ServiceAccount** + **RBAC** (metrics + apps + core + API group `artemisbrake.amq.io`).  
4. **Secret** for Jolokia.  
5. **Deployment** for operator.  
6. **NetworkPolicy** if you use default-deny.  
7. **`BrokerMemoryBrake` CR** with correct URLs and thresholds.  
8. **Verify** logs + CR status + broker behaviour.
