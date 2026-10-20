# crossplane-provider-purelymail

A native Go [Crossplane](https://crossplane.io) provider for [PurelyMail](https://purelymail.com): manage your mail
domains, mailboxes and routing rules (aliases, catch-alls, forwards) as Kubernetes resources, directly against the
PurelyMail REST API. No Terraform or Upjet in the loop.

| Kind | API group | What it manages |
|---|---|---|
| `PurelymailDomain` | `mail.purelymail.crossplane.io/v1alpha1` | A domain registered with your account |
| `PurelymailUser` | `mail.purelymail.crossplane.io/v1alpha1` | A mailbox (`user@domain`) |
| `PurelymailRoutingRule` | `mail.purelymail.crossplane.io/v1alpha1` | A routing rule: alias, prefix match or catch-all |
| `ProviderConfig` | `purelymail.crossplane.io/v1alpha1` | Credentials |

Package: `ghcr.io/aemendes/provider-purelymail-pkg`

> **Status:** `v1alpha1`, used in production by the author.

## Contents

- [Requirements](#requirements)
- [Installation](#installation)
- [Configuration](#configuration)
- [Resources](#resources)
  - [PurelymailDomain](#purelymaildomain)
  - [PurelymailUser](#purelymailuser)
  - [PurelymailRoutingRule](#purelymailroutingrule)
- [Bringing existing mail setup under management](#bringing-existing-mail-setup-under-management)
- [Behaviour and limitations](#behaviour-and-limitations)
- [Development](#development)
- [Release process](#release-process)
- [Security](#security) · [License](#license)

## Requirements

- A Kubernetes cluster with **Crossplane v1.17+ or v2.x**
  ([install guide](https://docs.crossplane.io/latest/software/install/)).
- A PurelyMail account and its **API token**: *purelymail.com → Manage → Account → API token*. PurelyMail has one
  token per account with full access (no scopes), so treat it like the account password.

## Installation

The package is public on GitHub Container Registry. Pick a version from the
[packages page](https://github.com/aemendes?tab=packages&repo_name=crossplane-provider-purelymail) (tags are
`v0.1.<build>`); pinning by digest is recommended.

```yaml
apiVersion: pkg.crossplane.io/v1
kind: Provider
metadata:
  name: provider-purelymail
spec:
  package: ghcr.io/aemendes/provider-purelymail-pkg:v0.1.X   # or @sha256:<digest>
```

```shell
kubectl apply -f provider.yaml
kubectl get providers.pkg.crossplane.io provider-purelymail   # INSTALLED=True, HEALTHY=True
```

Flags (set through a `DeploymentRuntimeConfig`, see the
[Crossplane docs](https://docs.crossplane.io/latest/packages/providers/#runtime-configuration)):

| Flag | Default | Description |
|---|---|---|
| `--poll` | `10m` | How often each resource is checked for drift. PurelyMail documents no rate limits; keep it modest. |
| `--sync` | `1h` | Controller manager resync period. |
| `--max-reconcile-rate` | `10` | Maximum reconciles per second. |
| `--leader-election` | `false` | Leader election, for more than one replica. |
| `--health-addr` | `:8081` | `/healthz` address. |
| `--debug` | `false` | Debug logging. |

## Configuration

Store the API token in a Secret (keep it out of git in plain text, e.g. with
[Sealed Secrets](https://github.com/bitnami-labs/sealed-secrets) or [External Secrets](https://external-secrets.io)),
and point a `ProviderConfig` at it. Resources use the `ProviderConfig` named `default` unless they set
`spec.providerConfigRef.name`.

```shell
kubectl -n crossplane-system create secret generic purelymail-creds --from-literal=credentials='<api-token>'
```

```yaml
apiVersion: purelymail.crossplane.io/v1alpha1
kind: ProviderConfig
metadata:
  name: default
spec:
  credentials:
    source: Secret
    secretRef:
      namespace: crossplane-system
      name: purelymail-creds
      key: credentials
```

All resources are cluster-scoped Crossplane managed resources and support `providerConfigRef`, `deletionPolicy`
and `managementPolicies`.

## Resources

### PurelymailDomain

| Field (`spec.forProvider`) | Type | Required | Description |
|---|---|---|---|
| `domainName` | string | yes | Domain, e.g. `example.com`. Immutable (it is also the external name). |
| `allowAccountReset` | bool | no | Let users on this domain reset their account through a recovery method. Unset leaves PurelyMail's current value alone. |
| `symbolicSubaddressing` | bool | no | Enable `user+tag@domain` addresses. Unset leaves PurelyMail's current value alone. |

| Status (`status.atProvider`) | Description |
|---|---|
| `isShared` | The domain was shared with you by another PurelyMail account. |
| `dnsSummary.passesMx` / `passesSpf` / `passesDkim` / `passesDmarc` | PurelyMail's last DNS check for the domain. |

**Before creating it**, publish PurelyMail's ownership TXT record at the domain apex; PurelyMail rejects the domain
otherwise. Get your account's ownership code with:

```shell
curl -s -X POST https://purelymail.com/api/v0/getOwnershipCode \
  -H "Purelymail-Api-Token: $PURELYMAIL_API_TOKEN" -d '{}'
```

```yaml
apiVersion: mail.purelymail.crossplane.io/v1alpha1
kind: PurelymailDomain
metadata:
  name: example-com
spec:
  forProvider:
    domainName: example.com
    allowAccountReset: true
    symbolicSubaddressing: true
```

### PurelymailUser

A mailbox. The password comes from a Kubernetes Secret, never from the resource itself.

| Field (`spec.forProvider`) | Type | Required | Description |
|---|---|---|---|
| `username` | string | yes | Local part, e.g. `alice` for `alice@example.com`. Immutable. |
| `domainName` | string | yes | Domain of the mailbox; must already exist in PurelyMail. Immutable. |
| `passwordSecretRef` | `{name, namespace, key}` | yes | Secret key holding the password. Used **at creation only**. |
| `enableSearchIndexing` | bool | no | Index mailbox contents for search. Kept in sync. |
| `requireTwoFactorAuthentication` | bool | no | Force the user to set up 2FA. Kept in sync. |
| `enablePasswordReset` | bool | no | Allow self-service password reset. Write-only (set at creation). |
| `recoveryEmail` / `recoveryEmailDescription` | string | no | Recovery email and its label. Write-only. |
| `recoveryPhone` / `recoveryPhoneDescription` | string | no | Recovery phone and its label. Write-only. |
| `sendWelcomeEmail` | bool | no | Send PurelyMail's welcome email on creation. |

| Status (`status.atProvider`) | Description |
|---|---|
| `recoveryEnabled` | Whether account recovery is possible. |
| `enableSpamFiltering` | Whether spam filtering is on (read-only in the API). |
| `resetMethods[]` | Recovery methods on file: `type`, `target`, `description`, `allowMfaReset`. |

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: alice-password
  namespace: crossplane-system
stringData:
  password: <a strong password>
---
apiVersion: mail.purelymail.crossplane.io/v1alpha1
kind: PurelymailUser
metadata:
  name: alice-example-com
spec:
  forProvider:
    username: alice
    domainName: example.com
    passwordSecretRef:
      namespace: crossplane-system
      name: alice-password
      key: password
    enableSearchIndexing: true
    requireTwoFactorAuthentication: true
```

### PurelymailRoutingRule

Routes mail for an address (or prefix, or every unclaimed address) on a domain to one or more targets.

| Field (`spec.forProvider`) | Type | Required | Description |
|---|---|---|---|
| `domainName` | string | yes | Domain the rule applies to. |
| `matchUser` | string | no | Local part to match (or prefix, with `prefix: true`). Ignored for catch-alls. |
| `prefix` | bool | no | Treat `matchUser` as a prefix. |
| `catchall` | bool | no | Match every address on the domain not claimed by a mailbox or a more specific rule. |
| `targetAddresses` | list of strings | yes (≥ 1) | Where matching mail is delivered. |

| Status | Description |
|---|---|
| `status.atProvider.id` | PurelyMail's numeric rule id (also the external name). |

```yaml
# Alias: hello@example.com -> alice@example.com
apiVersion: mail.purelymail.crossplane.io/v1alpha1
kind: PurelymailRoutingRule
metadata:
  name: hello-example-com
spec:
  forProvider:
    domainName: example.com
    matchUser: hello
    targetAddresses: [alice@example.com]
---
# Catch-all for the whole domain
apiVersion: mail.purelymail.crossplane.io/v1alpha1
kind: PurelymailRoutingRule
metadata:
  name: catchall-example-com
spec:
  forProvider:
    domainName: example.com
    catchall: true
    targetAddresses: [alice@example.com]
```

More examples in [`examples/`](examples/).

## Bringing existing mail setup under management

Existing domains, users and rules are matched by name (domain name, full address, or the rule's exact field set).
To adopt safely:

1. Declare the resources with `managementPolicies: ["Observe", "LateInitialize"]`.
2. Check `kubectl get managed` shows `READY=True`, `SYNCED=True`, and `kubectl describe` reports no drift.
3. Remove `managementPolicies` to give the provider full control.

Use `deletionPolicy: Orphan` on anything you never want deleted from PurelyMail when its Kubernetes resource goes
away (a mailbox, for instance).

## Behaviour and limitations

- **Passwords are applied at creation only.** PurelyMail never returns a password, so drift can't be detected and
  the provider does not resend it on unrelated updates. Rotate it in PurelyMail directly, or delete and recreate
  the resource.
- **Write-only user fields** (`enablePasswordReset`, `recovery*`, `sendWelcomeEmail`) are not returned by
  PurelyMail, so changes to them after creation are not applied or detected. `enableSearchIndexing` and
  `requireTwoFactorAuthentication` are kept in sync.
- **Renames are not supported**: `domainName` and `username` are immutable; recreate instead.
- **Routing rules have no update API.** Changing a rule deletes it and creates a new one. PurelyMail allows
  duplicate rules; if more than one existing rule matches the same fields, the provider reports an error instead
  of guessing which one it owns.
- **Kinds are prefixed `Purelymail*`** so they never collide with other CRDs named `users` or `domains`.
- **Not modelled:** app passwords (the API offers no way to read them back), billing.

### Notes on PurelyMail's API (for contributors)

Every operation, reads included, is `POST /api/v0/<verb>`, and the HTTP status is always `200`. Responses are a
union on `"type"`: `"success"` with a `"result"`, or `"error"` with `"code"`/`"message"` (see `do()` in
`internal/clients/purelymail/client.go`). There is no not-found code: unknown objects come back as
`internalError` with a message prefix (`Unknown user `, `Unknown domain "`, `Unknown rule `), which
`IsUserNotFound`/`IsDomainNotFound`/`IsRoutingRuleNotFound` match exactly, so unrelated internal errors still
surface. Domains have no per-domain read (the provider lists and filters), `createUser` takes a split
`userName`+`domainName` while every other call takes the full address, and `createRoutingRule` does not return the
new rule's id (it is recovered by listing and matching the full field set).

## Development

```shell
make generate   # regenerate deepcopy code and package/crds/*.yaml
make build
make test
```

| Path | Contents |
|---|---|
| `apis/v1alpha1` | `ProviderConfig` / `ProviderConfigUsage` (`purelymail.crossplane.io`) |
| `apis/{domain,user,routingrule}/v1alpha1` | managed resource types |
| `internal/clients/purelymail` | minimal `net/http` client for `https://purelymail.com/api/v0` |
| `internal/controller/*` | `Observe` / `Create` / `Update` / `Delete` per kind |
| `cmd/provider` | manager entrypoint |
| `package/` | Crossplane package metadata and generated CRDs |
| `examples/` | one manifest per kind |

Run against a cluster from your machine: `kubectl apply -f package/crds && go run ./cmd/provider --debug`.

## Release process

[`.github/workflows/build.yaml`](.github/workflows/build.yaml) builds, vets, tests and scans every push and pull
request. On `main` it pushes the runtime image and the Crossplane package to GHCR as `v0.1.<run number>` and prints
the digest. Optionally it sends a `pin-package` repository dispatch (`client_payload: {image, ref}`) to the
repositories in the `PIN_PACKAGE_REPOS` variable, using the `PIN_DISPATCH_TOKEN` secret (a fine-grained token with
Contents: Read and write on exactly those repositories).

## Security

See [SECURITY.md](SECURITY.md).

## License

[Apache License 2.0](LICENSE).
