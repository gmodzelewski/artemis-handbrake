> **Canonical copy:** this file lives under `/Users/gmodzele/workspace/gmodzelewski/artemis-handbrake/docs/OPERATOR-SOURCE.md`.

# artemis-handbrake — Go operator implementation (drop-in)

**Go module:** `github.com/gmodzelewski/artemis-handbrake`. Materialize the `.go` files from the sections below into this repository root (same folder as `README.md`).

## Layout

```
artemis-handbrake/
  go.mod
  main.go
  api/v1alpha1/groupversion_info.go
  api/v1alpha1/brokermemorybrake_types.go
  api/v1alpha1/zz_generated.deepcopy.go
  controllers/brokermemorybrake_controller.go
  pkg/jolokia/client.go
  Dockerfile
  Makefile
  config/crd/bases/artemisbrake.amq.io_brokermemorybrakes.yaml
  config/rbac/role.yaml
  config/rbac/role_binding.yaml
  config/rbac/service_account.yaml
  config/manager/manager.yaml
  config/samples/artemisbrake_v1alpha1_brokermemorybrake.yaml
```

After files exist, run:

```bash
cd artemis-handbrake
go mod tidy
make test  # if Makefile includes test
docker build -t artemis-handbrake:latest .
```

Generate deepcopy (optional, if you replace zz file):

```bash
go run sigs.k8s.io/controller-tools/cmd/controller-gen@v0.17.1 object:headerFile="hack/boilerplate.go.txt" paths="./api/..."
```

If you omit boilerplate, use the provided `zz_generated.deepcopy.go` as-is.

---

## go.mod

```go
module github.com/gmodzelewski/artemis-handbrake

go 1.23.0

require (
	k8s.io/api v0.32.2
	k8s.io/apimachinery v0.32.2
	k8s.io/client-go v0.32.2
	k8s.io/metrics v0.32.2
	sigs.k8s.io/controller-runtime v0.20.2
)
```

Run `go mod tidy` to fill `require` indirect blocks.

---

## api/v1alpha1/groupversion_info.go

```go
package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

var (
	GroupVersion = schema.GroupVersion{Group: "artemisbrake.amq.io", Version: "v1alpha1"}
	SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}
	AddToScheme = SchemeBuilder.AddToScheme
)
```

---

## api/v1alpha1/brokermemorybrake_types.go

```go
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BrokerMemoryBrake pauses an Artemis address (no new producer credit) when
// watched Deployment pods exceed a memory limit ratio, and resumes below resumeThreshold.
type BrokerMemoryBrake struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BrokerMemoryBrakeSpec   `json:"spec,omitempty"`
	Status BrokerMemoryBrakeStatus `json:"status,omitempty"`
}

type BrokerMemoryBrakeSpec struct {
	// TargetDeployment is the Deployment name in TargetNamespace whose Pods are measured.
	TargetDeployment string `json:"targetDeployment"`
	// TargetNamespace defaults to the CR namespace if empty.
	TargetNamespace string `json:"targetNamespace,omitempty"`

	// PauseThresholdPercent triggers Jolokia pause when max(pod memory/limit) >= this (0-100).
	PauseThresholdPercent int32 `json:"pauseThresholdPercent"`
	// ResumeThresholdPercent triggers resume when max ratio <= this (must be < pause).
	ResumeThresholdPercent int32 `json:"resumeThresholdPercent"`

	// JolokiaBaseURL e.g. https://amq-broker:8161/console/jolokia (no trailing slash required).
	JolokiaBaseURL string `json:"jolokiaBaseURL"`
	// JolokiaOrigin header if required by broker CORS (optional).
	JolokiaOrigin string `json:"jolokiaOrigin,omitempty"`
	// BrokerName is the JMX broker name segment (e.g. 0.0.0.0).
	BrokerName string `json:"brokerName"`
	// ArtemisAddress is the address name to pause/resume.
	ArtemisAddress string `json:"artemisAddress"`

	// CredentialsSecret is the name of a Secret in the CR namespace with keys username/password.
	CredentialsSecret string `json:"credentialsSecret"`
	UsernameKey       string `json:"usernameKey,omitempty"`
	PasswordKey       string `json:"passwordKey,omitempty"`

	// PollInterval is how often to re-evaluate metrics (e.g. 30s).
	// +kubebuilder:default=30
	// +kubebuilder:validation:Minimum=10
	PollIntervalSeconds int32 `json:"pollIntervalSeconds,omitempty"`
}

type BrokerMemoryBrakeStatus struct {
	ObservedGeneration int64  `json:"observedGeneration,omitempty"`
	LastMaxRatioPercent float64 `json:"lastMaxRatioPercent,omitempty"`
	LastPhase          string `json:"lastPhase,omitempty"` // idle|paused|error
	Message            string `json:"message,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

type BrokerMemoryBrakeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BrokerMemoryBrake `json:"items"`
}
```

Register types in `init()`:

```go
func init() {
	SchemeBuilder.Register(&BrokerMemoryBrake{}, &BrokerMemoryBrakeList{})
}
```

Add the `init` to `brokermemorybrake_types.go` bottom or `groupversion_info.go`.

---

## api/v1alpha1/zz_generated.deepcopy.go

Use `controller-gen object` or paste a minimal deepcopy from kubebuilder scaffold. Easiest: run operator-sdk/kubebuilder once, or copy from any v1alpha1 zz_generated file for two types + list.

---

## pkg/jolokia/client.go

```go
package jolokia

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	BaseURL  string
	User     string
	Password string
	Origin   string
	HTTP     *http.Client
}

func New(baseURL, user, password, origin string) *Client {
	b := strings.TrimSuffix(baseURL, "/")
	return &Client{
		BaseURL:  b,
		User:     user,
		Password: password,
		Origin:   origin,
		HTTP:     &http.Client{Timeout: 30 * time.Second},
	}
}

type jolokiaExecRequest struct {
	Type       string        `json:"type"`
	MBean      string        `json:"mbean"`
	Operation  string        `json:"operation"`
	Arguments  []interface{} `json:"arguments"`
}

type jolokiaResponse struct {
	Status int             `json:"status"`
	Error  string          `json:"error,omitempty"`
	Value  json.RawMessage `json:"value,omitempty"`
}

func addressMBean(brokerName, address string) string {
	return fmt.Sprintf(`org.apache.activemq.artemis:broker=%q,component=addresses,address=%q`, brokerName, address)
}

func (c *Client) Exec(operation, brokerName, address string) error {
	body, _ := json.Marshal(jolokiaExecRequest{
		Type:      "exec",
		MBean:     addressMBean(brokerName, address),
		Operation: operation,
		Arguments: []interface{}{},
	})
	req, err := http.NewRequest(http.MethodPost, c.BaseURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.Origin != "" {
		req.Header.Set("Origin", c.Origin)
	}
	if c.User != "" {
		req.SetBasicAuth(c.User, c.Password)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("jolokia http %d: %s", resp.StatusCode, string(b))
	}
	var jr jolokiaResponse
	if err := json.Unmarshal(b, &jr); err != nil {
		return fmt.Errorf("decode: %w body=%s", err, string(b))
	}
	if jr.Status != 200 {
		return fmt.Errorf("jolokia status %d: %s", jr.Status, jr.Error)
	}
	return nil
}

func (c *Client) IsPaused(brokerName, address string) (bool, error) {
	body, _ := json.Marshal(map[string]interface{}{
		"type":      "read",
		"mbean":     addressMBean(brokerName, address),
		"attribute": "Paused",
	})
	req, err := http.NewRequest(http.MethodPost, c.BaseURL, bytes.NewReader(body))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.Origin != "" {
		req.Header.Set("Origin", c.Origin)
	}
	if c.User != "" {
		req.SetBasicAuth(c.User, c.Password)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var jr struct {
		Status int             `json:"status"`
		Error  string          `json:"error,omitempty"`
		Value  json.RawMessage `json:"value"`
	}
	if err := json.Unmarshal(b, &jr); err != nil {
		return false, err
	}
	if jr.Status != 200 {
		return false, fmt.Errorf("jolokia: %s", jr.Error)
	}
	var paused bool
	_ = json.Unmarshal(jr.Value, &paused)
	return paused, nil
}
```

Note: Jolokia MBean string quoting must match Artemis expectations; some brokers want escaped quotes inside the JSON string differently—validate against your broker.

---

## controllers/brokermemorybrake_controller.go

Core logic:

1. Load CR; resolve namespace for deployment (`spec.targetNamespace` or `cr.Namespace`).
2. `Get` Deployment; list Pods with `client.MatchingLabels(dep.Spec.Selector.MatchLabels)`.
3. `Metrics().PodMetricses(ns).Get(ctx, podName, metav1.GetOptions{})` for each pod (or use metrics informer—simple loop is OK).
4. For each container, `limit := pod.Spec.Containers[i].Resources.Limits.Memory().Value()`; `usage := pm.Containers[j].Usage.Memory().Value()`; skip if limit==0; ratio = float64(usage)/float64(limit).
5. `maxRatio := max(ratios)*100`.
6. If `maxRatio >= pauseThreshold` → `jolokia.Exec("pause", ...)`.
7. Else if `maxRatio <= resumeThreshold` → `jolokia.Exec("resume", ...)`.
8. Else → no-op (hysteresis zone).
9. Patch status; `return ctrl.Result{RequeueAfter: time.Duration(spec.PollIntervalSeconds)*time.Second}`.

Wire `metrics.NewForConfig(mgr.GetConfig())` in `main.go` and pass `MetricsClient` into reconciler.

RBAC: `get,list,watch` on `pods`, `deployments`; `get` on `pods.metrics.k8s.io` (subresource `metrics` API: `podmetrics` resource).

---

## main.go

Standard controller-runtime `ctrl.NewManager`, `NewControllerManagedBy`, `For(&v1alpha1.BrokerMemoryBrake{})`, `Complete(&Reconciler{...})`, `mgr.Start`.

Scheme: `utilruntime.Must(clientgoscheme.AddToScheme(scheme)); utilruntime.Must(v1alpha1.AddToScheme(scheme))`.

---

## config/crd (kubebuilder annotations)

Generate with:

```bash
controller-gen crd:crdVersions=v1 paths=./api/... output:crd:dir=config/crd/bases
```

---

## Dockerfile

Multi-stage: `FROM golang:1.23 AS build` … `FROM gcr.io/distroless/static:nonroot`.

---

## OpenShift install notes

- CRD cluster-scoped or namespaced: namespaced CR is typical.
- Operator Deployment needs clusterRole for **metrics.k8s.io** `podmetrics` get/list in target namespaces—or run operator in same namespace and narrow RBAC.
- Jolokia URL must be reachable from operator pod (Service DNS).

---

To have the assistant **write these files automatically**, switch to **Agent mode** and say: “create the operator under ~/artemis-memory-brake-operator from the implementation doc”.

---

## api/v1alpha1/zz_generated.deepcopy.go (minimal manual)

```go
//go:build !ignore_autogenerated

package v1alpha1

import (
	runtime "k8s.io/apimachinery/pkg/runtime"
)

func (in *BrokerMemoryBrake) DeepCopyInto(out *BrokerMemoryBrake) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = in.Spec
	out.Status = in.Status
}

func (in *BrokerMemoryBrake) DeepCopy() *BrokerMemoryBrake {
	if in == nil {
		return nil
	}
	out := new(BrokerMemoryBrake)
	in.DeepCopyInto(out)
	return out
}

func (in *BrokerMemoryBrake) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func (in *BrokerMemoryBrakeList) DeepCopyInto(out *BrokerMemoryBrakeList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]BrokerMemoryBrake, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

func (in *BrokerMemoryBrakeList) DeepCopy() *BrokerMemoryBrakeList {
	if in == nil {
		return nil
	}
	out := new(BrokerMemoryBrakeList)
	in.DeepCopyInto(out)
	return out
}

func (in *BrokerMemoryBrakeList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}
```

Add to `brokermemorybrake_types.go` after imports:

```go
func init() {
	SchemeBuilder.Register(&BrokerMemoryBrake{}, &BrokerMemoryBrakeList{})
}
```

---

## controllers/brokermemorybrake_controller.go (full)

```go
package controllers

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsclient "k8s.io/metrics/pkg/client/clientset/versioned"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	artemisbrakev1alpha1 "github.com/gmodzelewski/artemis-handbrake/api/v1alpha1"
	"github.com/gmodzelewski/artemis-handbrake/pkg/jolokia"
)

type BrokerMemoryBrakeReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	MetricsClient *metricsclient.Clientset
}

// +kubebuilder:rbac:groups=artemisbrake.amq.io,resources=brokermemorybrakes,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=artemisbrake.amq.io,resources=brokermemorybrakes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=metrics.k8s.io,resources=pods,verbs=get;list

func (r *BrokerMemoryBrakeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var brake artemisbrakev1alpha1.BrokerMemoryBrake
	if err := r.Get(ctx, req.NamespacedName, &brake); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	targetNS := brake.Spec.TargetNamespace
	if targetNS == "" {
		targetNS = brake.Namespace
	}

	poll := time.Duration(brake.Spec.PollIntervalSeconds) * time.Second
	if poll < 10*time.Second {
		poll = 30 * time.Second
	}

	pauseTh := float64(brake.Spec.PauseThresholdPercent)
	resumeTh := float64(brake.Spec.ResumeThresholdPercent)
	if pauseTh <= 0 || pauseTh > 100 || resumeTh <= 0 || resumeTh >= pauseTh {
		brake.Status.LastPhase = "error"
		brake.Status.Message = "invalid pause/resume thresholds (pause must be > resume, both 1-100)"
		_ = r.Status().Update(ctx, &brake)
		return ctrl.Result{RequeueAfter: poll}, nil
	}

	var dep appsv1.Deployment
	depKey := types.NamespacedName{Namespace: targetNS, Name: brake.Spec.TargetDeployment}
	if err := r.Get(ctx, depKey, &dep); err != nil {
		if errors.IsNotFound(err) {
			brake.Status.LastPhase = "error"
			brake.Status.Message = fmt.Sprintf("deployment not found: %s/%s", targetNS, brake.Spec.TargetDeployment)
			_ = r.Status().Update(ctx, &brake)
			return ctrl.Result{RequeueAfter: poll}, nil
		}
		return ctrl.Result{}, err
	}

	var podList corev1.PodList
	sel := dep.Spec.Selector
	if sel == nil || len(sel.MatchLabels) == 0 {
		brake.Status.Message = "deployment has no selector"
		_ = r.Status().Update(ctx, &brake)
		return ctrl.Result{RequeueAfter: poll}, nil
	}
	if err := r.List(ctx, &podList, client.InNamespace(targetNS), client.MatchingLabels(sel.MatchLabels)); err != nil {
		return ctrl.Result{}, err
	}

	var maxRatio float64 = -1
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		pm, err := r.MetricsClient.MetricsV1beta1().PodMetricses(targetNS).Get(ctx, pod.Name, metav1.GetOptions{})
		if err != nil {
			logger.Info("pod metrics unavailable", "pod", pod.Name, "err", err.Error())
			continue
		}
		ratio := maxContainerMemoryRatio(&pod, pm)
		if ratio > maxRatio {
			maxRatio = ratio
		}
	}

	brake.Status.LastMaxRatioPercent = maxRatio * 100

	userKey, passKey := brake.Spec.UsernameKey, brake.Spec.PasswordKey
	if userKey == "" {
		userKey = "username"
	}
	if passKey == "" {
		passKey = "password"
	}
	var sec corev1.Secret
	secName := types.NamespacedName{Namespace: brake.Namespace, Name: brake.Spec.CredentialsSecret}
	if err := r.Get(ctx, secName, &sec); err != nil {
		brake.Status.LastPhase = "error"
		brake.Status.Message = fmt.Sprintf("credentials secret: %v", err)
		_ = r.Status().Update(ctx, &brake)
		return ctrl.Result{RequeueAfter: poll}, nil
	}
	user := string(sec.Data[userKey])
	pass := string(sec.Data[passKey])

	jc := jolokia.New(brake.Spec.JolokiaBaseURL, user, pass, brake.Spec.JolokiaOrigin)

	if maxRatio < 0 {
		brake.Status.LastPhase = "idle"
		brake.Status.Message = "no running pods with metrics yet"
		_ = r.Status().Update(ctx, &brake)
		return ctrl.Result{RequeueAfter: poll}, nil
	}

	pct := maxRatio * 100
	var act string
	if pct >= pauseTh {
		if err := jc.Exec("pause", brake.Spec.BrokerName, brake.Spec.ArtemisAddress); err != nil {
			brake.Status.LastPhase = "error"
			brake.Status.Message = fmt.Sprintf("jolokia pause: %v", err)
			_ = r.Status().Update(ctx, &brake)
			return ctrl.Result{RequeueAfter: poll}, nil
		}
		act = "paused"
	} else if pct <= resumeTh {
		if err := jc.Exec("resume", brake.Spec.BrokerName, brake.Spec.ArtemisAddress); err != nil {
			brake.Status.LastPhase = "error"
			brake.Status.Message = fmt.Sprintf("jolokia resume: %v", err)
			_ = r.Status().Update(ctx, &brake)
			return ctrl.Result{RequeueAfter: poll}, nil
		}
		act = "resumed"
	} else {
		act = "hold"
		brake.Status.Message = fmt.Sprintf("memory %.1f%% in hysteresis band (pause>=%.0f resume<=%.0f)", pct, pauseTh, resumeTh)
		brake.Status.LastPhase = "idle"
		_ = r.Status().Update(ctx, &brake)
		return ctrl.Result{RequeueAfter: poll}, nil
	}

	brake.Status.LastPhase = act
	brake.Status.Message = fmt.Sprintf("memory max %.1f%% -> %s", pct, act)
	_ = r.Status().Update(ctx, &brake)
	return ctrl.Result{RequeueAfter: poll}, nil
}

func maxContainerMemoryRatio(pod *corev1.Pod, pm *metricsv1beta1.PodMetrics) float64 {
	var maxR float64 = -1
	for _, c := range pod.Spec.Containers {
		lim := c.Resources.Limits.Memory()
		if lim == nil || lim.Value() == 0 {
			continue
		}
		var usage int64
		for _, mc := range pm.Containers {
			if mc.Name == c.Name && mc.Usage.Memory() != nil {
				usage = mc.Usage.Memory().Value()
				break
			}
		}
		if usage <= 0 {
			continue
		}
		r := float64(usage) / float64(lim.Value())
		if r > maxR {
			maxR = r
		}
	}
	return maxR
}

func (r *BrokerMemoryBrakeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&artemisbrakev1alpha1.BrokerMemoryBrake{}).
		Complete(r)
}
```

---

## main.go (full)

```go
package main

import (
	"flag"
	"os"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	metricsclient "k8s.io/metrics/pkg/client/clientset/versioned"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	artemisbrakev1alpha1 "github.com/gmodzelewski/artemis-handbrake/api/v1alpha1"
	"github.com/gmodzelewski/artemis-handbrake/controllers"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(artemisbrakev1alpha1.AddToScheme(scheme))
}

func main() {
	var metricsAddr, probeAddr string
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "Prometheus metrics bind address")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "health probe bind address")
	opts := zap.Options{Development: false}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	cfg, err := ctrl.GetConfig()
	if err != nil {
		setupLog.Error(err, "kubeconfig")
		os.Exit(1)
	}

	mcl, err := metricsclient.NewForConfig(cfg)
	if err != nil {
		setupLog.Error(err, "metrics client")
		os.Exit(1)
	}

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     metricsAddr,
		HealthProbeBindAddress: probeAddr,
	})
	if err != nil {
		setupLog.Error(err, "manager")
		os.Exit(1)
	}

	if err = (&controllers.BrokerMemoryBrakeReconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		MetricsClient: mcl,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "controller")
		os.Exit(1)
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "healthz")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "readyz")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "run")
		os.Exit(1)
	}
}
```

---

## config/crd/bases/artemisbrake.amq.io_brokermemorybrakes.yaml

Generate with `controller-gen crd` from types, or apply this minimal CRD (adjust storage/version as needed):

Use `controller-gen crd paths=./api/...` after adding kubebuilder markers `//+kubebuilder:object:root=true` and `//+kubebuilder:subresource:status` on `BrokerMemoryBrake` (already in types snippet).

---

## config/rbac/role.yaml

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: artemis-handbrake-manager-role
rules:
  - apiGroups: ["artemisbrake.amq.io"]
    resources: ["brokermemorybrakes"]
    verbs: ["get", "list", "watch", "update", "patch"]
  - apiGroups: ["artemisbrake.amq.io"]
    resources: ["brokermemorybrakes/status"]
    verbs: ["get", "update", "patch"]
  - apiGroups: ["apps"]
    resources: ["deployments"]
    verbs: ["get", "list", "watch"]
  - apiGroups: [""]
    resources: ["pods", "secrets"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["metrics.k8s.io"]
    resources: ["pods"]
    verbs: ["get", "list"]
```

For single-namespace operators, prefer **Role** + **RoleBinding** in one namespace and duplicate RoleBindings per watched namespace, or use **ClusterRole** with narrow rules as above.

---

## config/manager/manager.yaml (sketch)

Deployment `replicas: 1`, `serviceAccountName`, image `artemis-memory-brake-operator:latest`, args for probes, `WATCH_NAMESPACE` if you use operator-sdk pattern (not required for cluster-wide manager).

---

## Makefile

```makefile
IMG ?= artemis-handbrake:latest

.PHONY: build
build:
	go build -o bin/manager .

.PHONY: docker-build
docker-build:
	docker build -t $(IMG) .
```

---

## Dockerfile

```dockerfile
FROM golang:1.23 AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /manager .

FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /manager .
USER 65532:65532
ENTRYPOINT ["/manager"]
```

---

## Jolokia MBean string note

If `Exec` fails with MBean not found, dump available MBeans once from a debug pod and align `brokerName` / `address` with your broker.xml and address naming. Some installs use a different Jolokia path than `/console/jolokia`.

---

## config/samples/artemisbrake_v1alpha1_brokermemorybrake.yaml

```yaml
apiVersion: artemisbrake.amq.io/v1alpha1
kind: BrokerMemoryBrake
metadata:
  name: consumer-brake
  namespace: my-amq-namespace
spec:
  targetDeployment: my-consumer
  targetNamespace: my-amq-namespace
  pauseThresholdPercent: 80
  resumeThresholdPercent: 68
  pollIntervalSeconds: 30
  jolokiaBaseURL: https://amq-broker-hdls-svc:8161/console/jolokia
  jolokiaOrigin: "https://amq-broker-hdls-svc:8161"
  brokerName: "0.0.0.0"
  artemisAddress: "ORDERS"
  credentialsSecret: jolokia-admin
  usernameKey: username
  passwordKey: password
```

Create `Secret` `jolokia-admin` with keys `username` / `password` in the same namespace as the CR.

---

## Applying CRD

After `controller-gen crd ...`, run `oc apply -f config/crd/bases/`. Install RBAC and manager Deployment in the target namespace.

---

## Plan mode note

Cursor **plan mode** blocked writing `.go` files directly to disk. This document is the full drop-in. **Switch to Agent mode** (approve the mode change) and ask to materialize `~/artemis-handbrake` (or your clone path) from this file, or copy the sections into a new repo and run `go mod tidy` + `controller-gen crd`.
