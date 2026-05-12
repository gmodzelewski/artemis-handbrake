# Test deployment (OpenShift)

1. **Apply** (creates namespace, BCs, workloads, monitoring objects):

   ```bash
   oc apply -k .
   ```

2. **Build images** (from repository root):

   ```bash
   oc start-build handbrake-webhook --from-dir=../.. --follow --wait -n artemis-handbrake-test
   oc start-build mock-jolokia --from-dir=../.. --follow --wait -n artemis-handbrake-test
   ```

3. **Smoke** synthetic Alertmanager POST:

   ```bash
   ../../scripts/smoketest.sh artemis-handbrake-test
   oc logs -n artemis-handbrake-test deploy/mock-jolokia --tail=5
   ```

**Notes**

- `mock-jolokia` returns Jolokia-shaped JSON; replace with a real AMQ Broker Service URL in production.
- Namespace label `openshift.io/cluster-monitoring=true` may be rejected on some clusters; if `PrometheusRule` does not evaluate, ask your platform team to enable user workload monitoring for this namespace.
- `AlertmanagerConfig` routes `WorkloadMemoryHigh|WorkloadMemoryLow` to the webhook Service.
