# Railway GraphQL API

Endpoint: `https://backboard.railway.com/graphql/v2`

## Authentication

The tool resolves authentication in order of precedence:

1. **`--token` flag** — highest priority, for one-off commands.
2. **`RAILWAY_API_TOKEN` env var** — account/workspace-scoped. Uses
   `Authorization: Bearer` header.
3. **`RAILWAY_TOKEN` env var** — project-scoped. Uses
   `Project-Access-Token` header. Implicitly sets project + environment.
4. **Stored OAuth credentials** — `fat-controller auth login` performs a
   browser-based OAuth 2.0 flow and persists the token locally.

**Project access tokens** use the `Project-Access-Token` header and
implicitly scope to one project + environment. The `projectToken` query
returns the project and environment IDs.

**Account-level tokens** (from OAuth or manually created in the dashboard)
use the `Authorization: Bearer` header and can access any resource the user
is authorized for. Commands that need a project/environment will require
`--project` and `--environment` flags (or a local context file, future).

### OAuth 2.0 flow (auth login)

Railway exposes a full OAuth 2.0 + OIDC system:

- Authorization endpoint: `https://backboard.railway.com/oauth/auth`
- Token endpoint: `https://backboard.railway.com/oauth/token`
- Dynamic client registration: `POST https://backboard.railway.com/oauth/register`
- OIDC discovery: `https://backboard.railway.com/oauth/.well-known/openid-configuration`

The login flow:

1. Register as a native (public) client via dynamic registration if needed
   (one-time, client ID stored locally).
2. Start a local HTTP server on a random port for the callback.
3. Open the browser to the authorization endpoint with PKCE (`S256`),
   redirect URI `http://127.0.0.1:<port>/callback`.
4. Exchange the authorization code for an access token + refresh token.
5. Store tokens in OS keychain (primary) or fallback file (see
   [CONFIGURATION.md](CONFIGURATION.md)).
6. Use the refresh token to renew the access token transparently (1hr TTL).

`auth logout` clears stored tokens from keychain and fallback file.
`auth status` calls the `me` query and displays the authenticated user +
available scopes.

## Queries for get (live state)

All data needed for fetching live state is available via GQL — no Railway CLI dependency.

| Query | Returns |
|-------|---------|
| `projectToken` | Project ID + environment ID from the token |
| `project(id).services` | All service names + IDs |
| `project(id).volumes` | All volumes |
| `variables(projectId, environmentId, unrendered: true)` | Shared variables (unrendered preserves `${{ref}}` syntax) |
| `variables(projectId, environmentId, serviceId, unrendered: true)` | Per-service variables |
| `serviceInstance(environmentId, serviceId)` | Service config + domains + `latestDeployment.meta` |
| `serviceInstanceLimitOverride(environmentId, serviceId)` | Resource limits (CPU, memory) |
| `tcpProxies(serviceId, environmentId)` | TCP proxy config |

**Important nuance**: `serviceInstance` returns `null` for fields set via
`railway.toml` (e.g. healthcheck, watch patterns). The *effective* merged
values are in `latestDeployment.meta.serviceManifest`. Pull uses the manifest
for the state snapshot.

## Mutations for apply

| Mutation | Input type | Purpose |
|----------|-----------|---------|
| `variableCollectionUpsert` | `VariableCollectionUpsertInput` | Atomically set all variables for a service or shared. Has `replace: bool` (true = delete vars not in the set) and `skipDeploys: bool`. |
| `serviceInstanceUpdate` | `ServiceInstanceUpdateInput` | Update deploy/build settings: builder, dockerfilePath, rootDirectory, region, numReplicas, healthcheckPath/Timeout, restartPolicy, startCommand, preDeployCommand, cronSchedule, sleepApplication, watchPatterns, etc. |
| `serviceInstanceLimitsUpdate` | `ServiceInstanceLimitsUpdateInput` | Update resource limits: `vCPUs: Float`, `memoryGB: Float`. |

## Reference implementation

The [community Terraform provider](https://github.com/terraform-community-providers/terraform-provider-railway)
uses `genqlient` against the same API. Its `internal/provider/` directory
contains per-resource `.graphql` files with exact queries, mutations, and
`genqlient` annotations.
