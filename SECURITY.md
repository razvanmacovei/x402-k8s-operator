# Security Policy

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| 0.x.x   | :white_check_mark: |

## Reporting a Vulnerability

If you discover a security vulnerability in x402-k8s-operator, please report it responsibly.

**Do NOT open a public GitHub issue for security vulnerabilities.**

Instead, please send a report to:

- GitHub Security Advisory: [Report a vulnerability](https://github.com/razvanmacovei/x402-k8s-operator/security/advisories/new)

### What to include

- Description of the vulnerability
- Steps to reproduce
- Potential impact
- Suggested fix (if any)

### Response timeline

- **Acknowledgment**: Within 48 hours
- **Initial assessment**: Within 1 week
- **Fix and disclosure**: Coordinated with reporter

## Security Considerations

This operator handles payment routing in Kubernetes clusters. Key security considerations:

- The operator requires RBAC permissions to patch Ingress resources and manage Services
- Payment verification is delegated to an external facilitator service
- Wallet addresses and payment configuration are stored in CRD specs (not Secrets)
- The gateway proxy runs as a non-root user in a distroless container
- No secrets or private keys are stored by the operator
