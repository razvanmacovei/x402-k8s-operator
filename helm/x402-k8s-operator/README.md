# x402-k8s-operator Helm Chart

Kubernetes operator that monetizes any API with per-request payments via the [x402 protocol](https://x402.org).

Single binary, single Deployment. Patches your **existing Ingress** to enforce payment on specified paths. Works with any Ingress controller (NGINX, Traefik, etc.).

## Install

```bash
helm install x402-k8s-operator oci://ghcr.io/razvanmacovei/charts/x402-k8s-operator
```

With custom values:

```bash
helm install x402-k8s-operator oci://ghcr.io/razvanmacovei/charts/x402-k8s-operator \
  --set replicas=2 \
  --set leaderElection.enabled=true
```

## Usage

After installing the operator, create an `X402Route` to enable payment gating on an existing Ingress:

```yaml
apiVersion: x402.io/v1alpha1
kind: X402Route
metadata:
  name: my-api-payments
spec:
  ingressRef:
    name: my-api-ingress
  payment:
    wallet: "0xYourWalletAddress"
    network: base-sepolia        # or "base" for mainnet
    defaultPrice: "0.001"        # in USDC
  routes:
    - path: "/api/*"             # paid (inherits defaultPrice)
    - path: "/api/v2/**"
      price: "0.01"             # premium pricing
    - path: "/health"
      free: true                # no payment required
```

### Conditional Mode (humans free, bots pay)

```yaml
routes:
  - path: "/"
    mode: conditional
    conditions:
      - header: "User-Agent"
        pattern: "(?i)(claude|openai|anthropic|gpt|bot)"
        action: pay
      - header: "User-Agent"
        pattern: ".*"
        action: free
```

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `replicas` | int | `1` | Number of operator pod replicas |
| `image.repository` | string | `ghcr.io/razvanmacovei/x402-k8s-operator` | Container image repository |
| `image.tag` | string | `""` | Container image tag (defaults to chart `appVersion`) |
| `image.pullPolicy` | string | `IfNotPresent` | Image pull policy |
| `imagePullSecrets` | list | `[]` | Secrets for pulling from private registries |
| `nameOverride` | string | `""` | Override the chart name |
| `fullnameOverride` | string | `""` | Override the full release name |
| `namespace` | string | `x402-system` | Namespace to deploy into |
| `createNamespace` | bool | `true` | Create the namespace (set `false` if managed externally) |
| `rbac.create` | bool | `true` | Create RBAC resources |
| `leaderElection.enabled` | bool | `false` | Enable leader election (required for replicas > 1) |
| `podSecurityContext` | object | `{runAsNonRoot: true, runAsUser: 65532, ...}` | Pod-level security context |
| `securityContext` | object | `{allowPrivilegeEscalation: false, readOnlyRootFilesystem: true, ...}` | Container-level security context |
| `resources.limits.cpu` | string | `500m` | CPU limit |
| `resources.limits.memory` | string | `128Mi` | Memory limit |
| `resources.requests.cpu` | string | `100m` | CPU request |
| `resources.requests.memory` | string | `64Mi` | Memory request |
| `nodeSelector` | object | `{}` | Node selector for pod scheduling |
| `tolerations` | list | `[]` | Tolerations for pod scheduling |
| `affinity` | object | `{}` | Affinity rules for pod scheduling |
| `topologySpreadConstraints` | list | `[]` | Topology spread constraints |
| `podAnnotations` | object | `{}` | Additional pod annotations |
| `podLabels` | object | `{}` | Additional pod labels |
| `terminationGracePeriodSeconds` | int | `10` | Termination grace period |
| `priorityClassName` | string | `""` | Pod priority class |
| `gateway.port` | int | `8402` | Gateway proxy port |
| `metrics.enabled` | bool | `true` | Enable Prometheus metrics on `:8080/metrics` |
| `serviceMonitor.enabled` | bool | `false` | Create Prometheus ServiceMonitor |
| `serviceMonitor.interval` | string | `30s` | Scrape interval |
| `serviceMonitor.labels` | object | `{}` | Additional ServiceMonitor labels |
| `grafanaDashboard.enabled` | bool | `false` | Create ConfigMap with Grafana dashboard (auto-discovered by sidecar) |
| `grafanaDashboard.labels` | object | `{}` | Additional labels for the dashboard ConfigMap |
| `grafanaDashboard.namespace` | string | `""` | Namespace for the ConfigMap (defaults to operator namespace) |

## Grafana Dashboard

![Grafana Dashboard](https://raw.githubusercontent.com/razvanmacovei/x402-k8s-operator/main/docs/images/grafana-dashboard.png)

A pre-built dashboard is available on [Grafana.com (ID: 24921)](https://grafana.com/grafana/dashboards/24921-x402-kubernetes-operator/).

To auto-provision it via the Grafana sidecar in kube-prometheus-stack:

```yaml
grafanaDashboard:
  enabled: true
  namespace: monitoring  # namespace where Grafana is deployed
```

This creates a ConfigMap with the label `grafana_dashboard: "1"` that the Grafana sidecar automatically detects and loads.

## Architecture

The operator runs three services in a single pod:

| Port | Service | Purpose |
|------|---------|---------|
| 8080 | `/metrics` | Prometheus metrics |
| 8081 | `/healthz`, `/readyz` | Health probes |
| 8402 | Gateway proxy | Payment-gated traffic routing |

Traffic flow:

```
Client → Ingress Controller → x402-operator :8402 → payment check → Backend
                                    ↓
                              402 if no payment (PAYMENT-REQUIRED header)
                              200 if paid (verify + settle via facilitator, PAYMENT-RESPONSE header)
```

### Payment Protocol (x402)

The operator implements the [x402 specification](https://github.com/coinbase/x402/blob/main/specs/x402-specification-v2.md) and is compatible with the official Coinbase CDP facilitator.

- **No payment**: Returns `402` with a JSON body and a `PAYMENT-REQUIRED` header (Base64-encoded JSON) containing the payment requirements — `resource` object, `accepts` array with amount in atomic units, and `extra` asset metadata.
- **With payment**: Client sends a `Payment-Signature` header (Base64-encoded JSON payload). The gateway decodes it and POSTs `{paymentPayload, paymentRequirements}` to the facilitator's `/verify` endpoint, then `/settle` on success. The response includes a `PAYMENT-RESPONSE` header (Base64-encoded settle result with transaction hash and network).
- Prices in the CRD are human-readable (e.g. `"0.001"` USDC) and automatically converted to atomic units (e.g. `"1000"` for 6-decimal USDC).

## CRD Reference

### X402Route Spec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `ingressRef.name` | string | yes | Ingress to patch |
| `ingressRef.namespace` | string | no | Ingress namespace (defaults to X402Route's namespace) |
| `payment.wallet` | string | yes | Wallet address to receive payments |
| `payment.network` | string | yes | Blockchain network (see [Networks](#networks) table) |
| `payment.defaultPrice` | string | no | Default price in USDC (e.g. `"0.001"`) |
| `payment.facilitatorURL` | string | no | Facilitator URL (defaults to `https://x402.org/facilitator`) |
| `routes[].path` | string | yes | Path pattern (`*` = one segment, `**` = any depth) |
| `routes[].price` | string | no | Price override for this path |
| `routes[].free` | bool | no | Mark path as free (no payment) |
| `routes[].mode` | string | no | `all-pay` (default) or `conditional` |
| `routes[].conditions[].header` | string | yes | HTTP header to inspect |
| `routes[].conditions[].pattern` | string | yes | Regex pattern to match |
| `routes[].conditions[].action` | string | yes | `pay` or `free` when matched |

### X402Route Status

| Field | Type | Description |
|-------|------|-------------|
| `ingressPatched` | bool | Ingress has been patched |
| `ready` | bool | Route is fully active |
| `activeRoutes` | int | Number of active route rules |

## Networks

| Network | Chain ID | USDC Contract |
|---------|----------|---------------|
| `base` | eip155:8453 | `0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913` |
| `base-sepolia` | eip155:84532 | `0x036CbD53842c5426634e7929541eC2318f3dCF7e` |
| `avalanche` | eip155:43114 | `0xB97EF9Ef8734C71904D8002F8b6Bc66Dd9c48a6E` |
| `avalanche-fuji` | eip155:43113 | `0x5425890298aed601595a70AB815c96711a31Bc65` |
| `solana` | solana:5eykt4UsFv8P8NJdTREpY1vzqKqZKvdp | `EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v` |
| `solana-devnet` | solana:EtWTRABZaYq6iMfeYKouRu166VU2xqa1 | `4zMMC9srt5Ri5X14GAgXhaHii3GnPAEERYPJgZJDncDU` |

## Uninstall

```bash
helm uninstall x402-k8s-operator
```

> **Note:** Uninstalling the chart automatically deletes all X402Route resources and restores patched Ingresses to their original backends via a pre-delete hook.
