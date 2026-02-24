# x402-k8s-operator

[![Go](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)
[![Artifact Hub](https://img.shields.io/endpoint?url=https://artifacthub.io/badge/repository/x402-k8s-operator)](https://artifacthub.io/packages/helm/x402-k8s-operator/x402-k8s-operator)
[![Release](https://img.shields.io/github/v/release/razvanmacovei/x402-k8s-operator)](https://github.com/razvanmacovei/x402-k8s-operator/releases)

**Kubernetes operator that monetizes any API with per-request payments via the [x402 protocol](https://x402.org).**

Single binary, single Deployment. Patches your **existing Ingress** to enforce payment on specified paths. Works with **any Ingress controller** (NGINX, Traefik, etc.) using only the standard `networking.k8s.io/v1` Ingress API.

```
                Internet
                   |
                   v
    +----------------------------+
    |  Your existing Ingress     |  <-- operator patches this
    |  (NGINX / Traefik / etc)   |
    +------+-----------+---------+
           |           |
     paid paths    free paths
           |           |
           v           v
    +-----------------+  +-----------------+
    | x402-k8s-       |  | Backend Service |
    | operator :8402  |  | (your API)      |
    | (payment check) |  +-----------------+
    +--------+--------+
             |
    402 if no payment
    forward if paid
             |
             v
    +-----------------+
    | Backend Service |
    | (your API)      |
    +-----------------+
```

---

## Quick Start

### 1. Install the operator

**kubectl** (single manifest):
```bash
kubectl apply -f https://raw.githubusercontent.com/razvanmacovei/x402-k8s-operator/main/install.yaml
```

**Helm**:
```bash
helm install x402-k8s-operator oci://ghcr.io/razvanmacovei/charts/x402-k8s-operator
```

### 2. Create an X402Route

Assuming you already have an Ingress called `my-api-ingress`:

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
    network: base-sepolia
    defaultPrice: "0.001"
  routes:
    - path: "/api/*"
      # inherits defaultPrice, all requests pay
    - path: "/health"
      free: true
    - path: "/docs/**"
      free: true
```

That's it. The operator automatically:
1. Compiles route rules into an in-memory store
2. Patches your Ingress: paid paths -> operator service, free paths -> original backend
3. Serves traffic on port 8402: checks payment -> verifies with facilitator -> proxies to backend

---

## CRD Reference

### Example: Per-path pricing with conditional mode

```yaml
apiVersion: x402.io/v1alpha1
kind: X402Route
metadata:
  name: my-api
spec:
  ingressRef:
    name: my-ingress
  payment:
    wallet: "0x..."
    network: base-sepolia
    defaultPrice: "0.001"
  routes:
    - path: "/api/v1/*"
      # inherits defaultPrice, mode defaults to all-pay
    - path: "/api/v2/**"
      price: "0.01"
      mode: conditional
      conditions:
        - header: "X-Bot-Score"
          pattern: "^(bot|automated)$"
          action: pay
        - header: "User-Agent"
          pattern: "(?i)(claude|openai|anthropic)"
          action: pay
    - path: "/health"
      free: true
    - path: "/docs/**"
      free: true
```

### Spec Fields

| Field | Type | Required | Description |
|---|---|---|---|
| `ingressRef.name` | `string` | yes | Name of the existing Ingress to patch |
| `ingressRef.namespace` | `string` | no | Namespace of the Ingress (defaults to X402Route's ns) |
| `payment.wallet` | `string` | yes | Wallet address to receive payments |
| `payment.network` | `string` | yes | Blockchain network: `base` or `base-sepolia` |
| `payment.defaultPrice` | `string` | no | Default price for paid routes (e.g. `"0.001"`) |
| `payment.facilitatorURL` | `string` | no | Facilitator URL (defaults to `https://x402.org/facilitator`) |
| `routes[].path` | `string` | yes | Path pattern (`*` = one segment, `**` = any depth) |
| `routes[].price` | `string` | no | Price override for this path |
| `routes[].free` | `bool` | no | Mark path as free |
| `routes[].mode` | `string` | no | `all-pay` (default) or `conditional` |
| `routes[].conditions[]` | `array` | no | Conditions for conditional mode |
| `routes[].conditions[].header` | `string` | yes | HTTP header to inspect |
| `routes[].conditions[].pattern` | `string` | yes | Regex pattern to match |
| `routes[].conditions[].action` | `string` | yes | `pay` or `free` when matched |

### Status Fields

| Field | Type | Description |
|---|---|---|
| `status.ingressPatched` | `bool` | Whether the Ingress has been patched |
| `status.ready` | `bool` | Whether the route is fully active |
| `status.activeRoutes` | `int` | Number of active route rules |
| `status.conditions` | `[]Condition` | Standard Kubernetes conditions |

---

## Architecture

Single pod runs both controller and HTTP proxy as goroutines:

| Port | Purpose |
|---|---|
| `:8080` | `/metrics` (Prometheus) |
| `:8081` | `/healthz`, `/readyz` (probes) |
| `:8402` | Gateway proxy (traffic) |

The controller watches X402Route CRDs and writes compiled routes to an **in-memory store**. The gateway reads from the store instantly — no ConfigMap polling, no separate Deployment.

### Traffic Flow

```
Client -> Ingress Controller -> x402-k8s-operator :8402 -> payment check -> Original Backend
```

### Payment Headers (x402 V2)

- **Request**: `Payment-Signature` header (falls back to `X-Payment` for V1 compat)
- **402 Response**: `Payment-Required: x402` header + JSON body with payment requirements
- **200 Response**: `Payment-Response: accepted` header

### Prometheus Metrics

| Metric | Type | Description |
|---|---|---|
| `x402_requests_total` | counter | Requests by path, namespace, route, payment status |
| `x402_payment_amount_total` | counter | Payment amounts by path, wallet, network |
| `x402_payment_verification_duration_seconds` | histogram | Facilitator verification latency |
| `x402_proxy_request_duration_seconds` | histogram | Backend proxy latency |
| `x402_active_routes` | gauge | Number of active routes |
| `x402_route_store_updates_total` | counter | Route store update count |

---

## Local Development

Prerequisites: Go 1.25+, Docker, a local Kubernetes cluster (docker-desktop, Kind, or Minikube).

```bash
# Build the manager binary
make build

# Build Docker image
make docker-build

# Deploy to local cluster
make deploy-local

# Apply sample X402Route
make sample

# Remove everything
make undeploy
```

---

## Testing

### With mock facilitator (no blockchain needed)

```bash
# Terminal 1: Start a simple backend
python3 -m http.server 9090

# Terminal 2: Start the mock facilitator
go run ./cmd/mock-facilitator/

# Terminal 3: Run the test client
go run ./cmd/test-client/ http://localhost:8402/api/hello
```

The test client sends two requests:
1. Without payment -> expects `402 Payment Required`
2. With a mock `Payment-Signature` header -> expects `200 OK`

---

## Production

For production, use **Base mainnet** with a real USDC wallet:

```yaml
payment:
  wallet: "0xYourProductionWallet"
  network: base
  defaultPrice: "0.01"
```

| Network | Chain | USDC Contract |
|---|---|---|
| `base` | Base mainnet | `0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913` |
| `base-sepolia` | Base Sepolia testnet | `0x036CbD53842c5426634e7929541eC2318f3dCF7e` |

---

## Contributing

Contributions are welcome! See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

Please read our [Code of Conduct](CODE_OF_CONDUCT.md) before participating.

## Security

For reporting security vulnerabilities, see [SECURITY.md](SECURITY.md).

## License

[Apache License 2.0](LICENSE)
