# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Development Commands

```bash
make build            # Build oauth2-proxy binary (injects version from git)
make test             # Run all tests
make test COVER=true  # Run tests with coverage profile
make lint             # Run golangci-lint
make generate         # Regenerate alpha config docs from Go structs
make clean            # Remove release artifacts and built binaries
```

Run a single test package:
```bash
GO111MODULE=on go test -v ./pkg/sessions/...
```

Run a specific test by name:
```bash
GO111MODULE=on go test -v -run TestFunctionName ./pkg/...
```

Local test environment (docker-compose based):
```bash
make local-env-up     # Start local environment
make local-env-down   # Stop local environment
```

## Architecture

oauth2-proxy is a reverse proxy that adds OAuth2/OIDC authentication in front of backend services. It can act as a standalone reverse proxy or as a forward auth middleware for other proxies (nginx, Traefik, etc.).

**Entry point:** `main.go` loads configuration (legacy TOML or alpha YAML), then `oauthproxy.go` contains the main `OAuthProxy` struct that handles the HTTP request lifecycle: session lookup → provider auth redirect → callback → upstream forwarding.

**Key packages under `pkg/`:**

- `providers/` — One file per OAuth2 provider (Google, GitHub, Azure, GitLab, OIDC generic, etc.). Each provider implements the `Provider` interface with `GetLoginURL`, `Redeem`, `RefreshSession`, and `ValidateSession`.
- `providers/oidc/` — OIDC-specific provider implementations and JWT verification.
- `sessions/` — Session storage backends: cookie (encrypted), Redis, and a persistence layer that combines them. The `SessionStore` interface is in `pkg/apis/`.
- `middleware/` — HTTP middleware chain assembled at startup. Handles request routing decisions (allow/deny/redirect) before the request reaches the upstream.
- `header/` — Injects user identity into request headers (e.g., `X-Auth-Request-User`, `X-Auth-Request-Email`) before forwarding to the upstream.
- `upstream/` — Reverse proxy to backend applications; supports HTTP and Unix socket upstreams.
- `cookies/` — Cookie creation, parsing, and CSRF token management.
- `encryption/` — AES-GCM encryption for cookie/session data.
- `apis/options/` — All configuration structs. Legacy options are in `options.go`; the alpha (YAML) format is in the `alpha/` subdirectory.
- `validation/` — Configuration validation called at startup.
- `app/` — Page rendering (sign-in page, error pages) and redirect URL validation.

**Configuration formats:** There are two config formats. The legacy format uses CLI flags/TOML. The alpha format is a YAML file passed with `--alpha-config`; it has a different struct tree in `pkg/apis/options/alpha/`. `--convert-config-to-alpha` migrates between them.

**Testing:** Tests use the Ginkgo v2 + Gomega BDD framework. Test suites are initialized with `RegisterFailHandler` / `RunSpecs` in `*_suite_test.go` files per package. The top-level `oauthproxy_test.go` contains large integration-style tests for the full request lifecycle.

## Stratio Fork — Differences from Upstream

This repo is a fork of [oauth2-proxy/oauth2-proxy](https://github.com/oauth2-proxy/oauth2-proxy), currently tracking **upstream v7.13.0**. Stratio-specific changes are layered on top via a set of commits starting from `e6593e8`. Do not rebase or squash those commits when bumping the upstream version — instead, cherry-pick or merge them on top of the new upstream tag.

### SIS Provider (`providers/sis.go`)

A custom OAuth2 provider for **Stratio Identity Server (SIS)**, a CAS-based SSO. Registered as provider type `"sis"`.

Key behaviours that differ from standard OIDC:
- Token endpoint returns either JSON or `application/x-www-form-urlencoded` (both parsed in `Redeem()`).
- User profile is fetched from `/sso/oauth2.0/profile` using a Bearer token and returns a custom `attributes` array, not standard OIDC claims. `EnrichSession()` maps `uid → User`, `cn → PreferredUsername`, `mail → Email`, `tenant → Tenant`, `username → Username`, `groups → Groups`, `tenants → Tenants`.
- Sign-out redirects to SIS's `/sso/logout?rd=<redirect>` via a custom `GetSignOutURL()`. The base `ProviderData.GetSignOutURL()` is a no-op passthrough added to the `Provider` interface so other providers are unaffected.
- All endpoint paths are derived from a single `--sis-root-url` flag (e.g. `https://sis.example.com/sso`). Individual login/redeem/profile URLs can still be overridden separately.
- `--clear-extra-cookie-names` (SIS-specific flag): list of extra cookie names to expire on sign-in, sign-out, and access-denied. This is needed because SIS sets its own session cookies that must be cleared on logout.

Configuration options (`pkg/apis/options/providers.go → SISOptions`):
```
--sis-root-url              Stratio SIS root URL (e.g. https://sis/sso)
--clear-extra-cookie-names  Comma-separated cookie names to clear on logout
```

### JWT Session Store (`pkg/sessions/jwt/`)

A new session store type (select with `--session-store-type=jwt`) that stores the session as an RSA-signed JWT cookie instead of an encrypted blob. Required when downstream services need to read the session directly from the cookie without calling oauth2-proxy.

JWT claims are Stratio-specific: `uid`, `cn`, `username`, `mail`, `tenant`, `groups`, `tenants`.

Configuration:
```
--jwt-session-key       RSA private key in PEM format (inline)
--jwt-session-key-file  Path to RSA private key PEM file
```

Fix applied on top of upstream: `Load()` returns `nil, nil` (instead of error) when the JWT cookie is missing — needed so the proxy correctly treats a missing cookie as "unauthenticated" rather than an error.

### Extended SessionState and UserInfo

`pkg/apis/sessions/session_state.go` adds three fields absent from upstream:
- `Tenant string` — primary tenant for the user
- `Username string` — SIS internal username (distinct from `User`/`uid`)
- `Tenants []string` — all tenants the user belongs to

The `/oauth2/userinfo` JSON response (`oauthproxy.go`) exposes these three fields alongside the standard `user`, `email`, `groups`, and `preferredUsername`.

### CI/CD Changes

- GitHub Actions workflows removed; replaced with a `Jenkinsfile` for Stratio's internal Jenkins/ECR pipeline.
- `VERSION` file drives semantic versioning; the Jenkins pipeline reads it instead of using git tags.
- `CHANGELOG.md` stripped to just Stratio entries to reduce noise in diffs.

## CI / Docker

The Jenkinsfile drives CI for Stratio's internal pipeline. The Docker image is built and pushed to ECR. To check what image a PR produced, use:

```bash
gh api repos/<org>/oauth2-proxy/commits/<sha>/check-runs \
  --jq '.check_runs[] | select(.name | contains("Docker")) | {name, output: .output}'
```

The Dockerfile uses a multi-stage build: `golang:1.25-bookworm` for compilation, then `distroless/static:nonroot` (default) or `alpine` for the runtime image. Supports linux/amd64, arm64, ppc64le, arm/v6, arm/v7, s390x.
