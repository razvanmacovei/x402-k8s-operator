# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.0.1] - 2026-02-23

### Added
- Single-binary Kubernetes operator with integrated payment gateway
- `X402Route` Custom Resource Definition (CRD) with per-path pricing
- Conditional payment mode with header-based regex matching
- x402 V2 protocol support (`Payment-Signature` / `Payment-Required` headers)
- Backward compatibility with x402 V1 (`X-Payment` header)
- Prometheus metrics: request counters, payment amounts, verification latency, proxy latency
- Helm chart with ArtifactHub annotations
- Works with any Ingress controller via standard `networking.k8s.io/v1` Ingress API
- Automated CI/CD: Docker image + Helm chart published on tag
- `install.yaml` for single-command kubectl installation
- Mock facilitator and test client for local E2E testing
