# Security policy

## Reporting a vulnerability

Please **do not open a public issue** for security problems. Report them privately through
[GitHub's private vulnerability reporting](https://github.com/aemendes/crossplane-provider-purelymail/security/advisories/new)
for this repository. Include the affected provider (`tunnel` or `access`), the version or package digest, and steps
to reproduce. You should get a first answer within a week.

## Scope notes

- The providers hold a Cloudflare API token (from the `ProviderConfig` Secret) and write tunnel tokens and SaaS
  OIDC client secrets to Kubernetes Secrets when `writeConnectionSecretToRef` is set. Anyone who can read those
  Secrets in your cluster can use them; restrict RBAC accordingly.
- Give the API token only the permissions listed in the README for the providers you run.
- Only the latest build of each provider receives fixes.
