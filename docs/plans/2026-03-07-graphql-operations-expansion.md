# GraphQL Operations Expansion

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Expand the Railway GraphQL client to cover all entities the architecture manages: service CRUD, domains, volumes, TCP proxies, private networks, egress gateways, deployment triggers, buckets, and expanded service instance fields.

**Architecture:** The Railway client uses genqlient for type-safe GraphQL operations. Operations are defined in `operations.graphql`, types are generated from `schema.graphql`. Hand-written wrapper functions in `mutate.go`, `state.go`, and `resolve.go` provide the public API. The client supports variable upsert/delete, service settings update, and service limits update. It needs expansion to cover all declarative entity types.

**Tech Stack:** Go 1.26, genqlient (Khan/genqlient), Railway GraphQL API v2.

**Can run in parallel with:** Plans 1 and 2 (no dependency). Plan 3 depends on this.

---

## Context for the implementer

### Current operations

**Queries (7):** ProjectToken, ApiToken, Projects, Environments, ProjectServices, Variables, ServiceInstance, ServiceInstanceLimits

**Mutations (5):** VariableUpsert, VariableDelete, VariableCollectionUpsert, ServiceInstanceUpdate, ServiceInstanceLimitsUpdate

### Missing operations (from schema review)

**Queries needed:**

| Query | Purpose |
|-------|---------|
| `domains(envId, projectId, serviceId)` | List domains for a service |
| `tcpProxies(envId, serviceId)` | List TCP proxies |
| `egressGateways(envId, serviceId)` | List egress gateways |
| `privateNetworks(envId)` | List private networks |
| `privateNetworkEndpoint(envId, networkId, serviceId)` | Get network endpoint |
| `deploymentTriggers(envId, projectId, serviceId)` | List deployment triggers |
| `deployments(input)` | List deployments |
| `deployment(id)` | Get single deployment |
| `volumeInstance(id)` | Get volume instance |
| `service(id)` | Full service detail |
| `regions(projectId)` | Available regions |
| `me` | Current user info |
| `deploymentLogs(deploymentId)` | Fetch deploy logs |
| `buildLogs(deploymentId)` | Fetch build logs |
| `environmentLogs(envId)` | Fetch environment logs |
| `bucketS3Credentials(...)` | Get bucket credentials |

**Mutations needed:**

| Mutation | Purpose |
|----------|---------|
| `serviceCreate(input)` | Create service |
| `serviceDelete(id)` | Delete service |
| `serviceUpdate(id, input)` | Update service name/icon |
| `serviceConnect(id, input)` | Connect service to repo/image |
| `projectCreate(input)` | Create project |
| `projectDelete(id)` | Delete project |
| `environmentCreate(input)` | Create environment |
| `environmentDelete(id)` | Delete environment |
| `customDomainCreate(input)` | Create custom domain |
| `customDomainDelete(id)` | Delete custom domain |
| `customDomainUpdate(...)` | Update custom domain port |
| `serviceDomainCreate(input)` | Create service domain |
| `serviceDomainDelete(id)` | Delete service domain |
| `tcpProxyCreate(input)` | Create TCP proxy |
| `tcpProxyDelete(id)` | Delete TCP proxy |
| `volumeCreate(input)` | Create volume |
| `volumeDelete(volumeId)` | Delete volume |
| `volumeInstanceUpdate(...)` | Update volume instance |
| `deploymentTriggerCreate(input)` | Create deployment trigger |
| `deploymentTriggerUpdate(id, input)` | Update trigger |
| `deploymentTriggerDelete(id)` | Delete trigger |
| `egressGatewayAssociationCreate(input)` | Create egress |
| `egressGatewayAssociationsClear(input)` | Clear egress |
| `privateNetworkCreateOrGet(input)` | Enable private network |
| `privateNetworkEndpointCreateOrGet(input)` | Create/get endpoint |
| `privateNetworkEndpointDelete(id)` | Delete endpoint |
| `serviceInstanceDeploy(envId, serviceId)` | Trigger deploy |
| `deploymentRedeploy(id)` | Redeploy |
| `deploymentRestart(id)` | Restart |
| `deploymentRollback(id)` | Rollback |
| `deploymentCancel(id)` | Cancel |
| `bucketCreate(input)` | Create bucket |
| `bucketUpdate(id, input)` | Update bucket |

### Key files

| File | Role |
|------|------|
| `internal/railway/operations.graphql` | GraphQL operations (add here) |
| `internal/railway/schema.graphql` | Full schema (read-only reference) |
| `.config/genqlient.yaml` | genqlient config |
| `internal/railway/generated.go` | Generated code (regenerated) |
| `internal/railway/mutate.go` | Hand-written mutation wrappers |
| `internal/railway/state.go` | Hand-written state-fetching functions |
| `internal/railway/resolve.go` | Name → ID resolution |

### Workflow to add operations

1. Add query/mutation to `operations.graphql`
2. Run `go generate ./internal/railway/`
3. Write hand-written wrapper in the appropriate file
4. Write tests (use the mock GraphQL server pattern from `e2e_mocked_graphql_test.go`)
5. Commit

---

## Task 1: Expand ServiceInstance query fields

The current `ServiceInstance` query only fetches 5 fields. Expand it
to fetch all fields the architecture needs.

**Files:**

- Modify: `internal/railway/operations.graphql`
- Modify: `internal/railway/state.go`
- Regenerate: `internal/railway/generated.go`

### Step 1: Update the ServiceInstance query

```graphql
query ServiceInstance($environmentId: String!, $serviceId: String!) {
  serviceInstance(environmentId: $environmentId, serviceId: $serviceId) {
    builder
    buildCommand
    startCommand
    dockerfilePath
    rootDirectory
    healthcheckPath
    healthcheckTimeout
    cronSchedule
    numReplicas
    region
    restartPolicyType
    restartPolicyMaxRetries
    drainingSeconds
    overlapSeconds
    sleepApplication
    ipv6EgressEnabled
    watchPatterns
    preDeployCommand
    source {
      image
      repo
    }
    domains {
      customDomains {
        id
        domain
        targetPort
        status {
          certificateStatus
          dnsRecords {
            fqdn
            hostlabel
            purpose
            requiredValue
            zone
          }
          verified
        }
      }
      serviceDomains {
        id
        domain
        targetPort
        suffix
      }
    }
  }
}
```

### Step 2: Regenerate

Run: `go generate ./internal/railway/`

### Step 3: Update state.go to populate expanded LiveConfig

Map the new query fields to the expanded `ServiceConfig` from Plan 1.

### Step 4: Run tests

### Step 5: Commit

---

## Task 2: Add domain operations

**Files:**

- Modify: `internal/railway/operations.graphql`
- Create: `internal/railway/domains.go` (hand-written wrappers)
- Test: `internal/railway/domains_test.go`

### Step 1: Add GraphQL operations

```graphql
mutation CustomDomainCreate($input: CustomDomainCreateInput!) {
  customDomainCreate(input: $input) {
    id
    domain
  }
}

mutation CustomDomainDelete($id: String!) {
  customDomainDelete(id: $id)
}

mutation ServiceDomainCreate($input: ServiceDomainCreateInput!) {
  serviceDomainCreate(input: $input) {
    id
    domain
  }
}

mutation ServiceDomainDelete($id: String!) {
  serviceDomainDelete(id: $id)
}
```

### Step 2: Regenerate

### Step 3: Write wrapper functions

```go
func CreateCustomDomain(ctx context.Context, client *Client, projectID, envID, serviceID, domain string, port int) (string, error)
func DeleteCustomDomain(ctx context.Context, client *Client, id string) error
func CreateServiceDomain(ctx context.Context, client *Client, envID, serviceID string, port int) (string, error)
func DeleteServiceDomain(ctx context.Context, client *Client, id string) error
```

### Step 4: Write tests

### Step 5: Commit

---

## Task 3: Add volume operations

**Files:**

- Modify: `internal/railway/operations.graphql`
- Create: `internal/railway/volumes.go`
- Test: `internal/railway/volumes_test.go`

### Step 1: Add GraphQL operations

VolumeCreate, VolumeDelete, VolumeInstanceUpdate queries.

### Step 2–5: Standard flow + commit

---

## Task 4: Add TCP proxy operations

### Step 1: Add TcpProxyCreate, TcpProxyDelete to operations.graphql

### Step 2–5: Standard flow + commit

---

## Task 5: Add private network operations

### Step 1: Add PrivateNetworkCreateOrGet, PrivateNetworkEndpointCreateOrGet, PrivateNetworkEndpointDelete

### Step 2–5: Standard flow + commit

---

## Task 6: Add egress gateway operations

### Step 1: Add EgressGatewayAssociationCreate, EgressGatewayAssociationsClear

### Step 2–5: Standard flow + commit

---

## Task 7: Add deployment trigger operations

### Step 1: Add DeploymentTriggerCreate, DeploymentTriggerUpdate, DeploymentTriggerDelete

### Step 2–5: Standard flow + commit

---

## Task 8: Add service CRUD operations

**Files:**

- Modify: `internal/railway/operations.graphql`
- Create: `internal/railway/services.go`
- Test: `internal/railway/services_test.go`

### Step 1: Add GraphQL operations

```graphql
mutation ServiceCreate($input: ServiceCreateInput!) {
  serviceCreate(input: $input) {
    id
    name
  }
}

mutation ServiceDelete($id: String!) {
  serviceDelete(id: $id)
}

mutation ServiceUpdate($id: String!, $input: ServiceUpdateInput!) {
  serviceUpdate(id: $id, input: $input)
}

mutation ServiceConnect($id: String!, $input: ServiceConnectInput!) {
  serviceConnect(id: $id, input: $input) {
    id
  }
}
```

### Step 2–5: Standard flow + commit

---

## Task 9: Add project and environment CRUD

### Step 1: Add ProjectCreate, ProjectDelete, EnvironmentCreate, EnvironmentDelete

### Step 2–5: Standard flow + commit

---

## Task 10: Add deployment lifecycle operations

### Step 1: Add ServiceInstanceDeploy, DeploymentRedeploy, DeploymentRestart, DeploymentRollback, DeploymentCancel

### Step 2–5: Standard flow + commit

---

## Task 11: Add bucket operations

### Step 1: Add BucketCreate, BucketUpdate, BucketS3Credentials query

### Step 2–5: Standard flow + commit

---

## Task 12: Add log fetching operations

### Step 1: Add DeploymentLogs, BuildLogs, EnvironmentLogs queries

### Step 2–5: Standard flow + commit

---

## Task 13: Add deployment listing

### Step 1: Add Deployments query (paginated)

Implement cursor-following pagination for deployments. Add a helper
that follows `hasNextPage` / `endCursor` to collect all results.

### Step 2–5: Standard flow + commit

---

## Task 14: Update apply engine for sub-resources

Expand the `Applier` interface and `RailwayApplier` to handle all
new entity types.

**Files:**

- Modify: `internal/apply/apply.go`
- Modify: `internal/apply/railway.go`
- Test: updated apply tests

### Step 1: Expand Applier interface

```go
type Applier interface {
	// Variables (existing)
	UpsertVariable(...) error
	UpsertVariables(...) error
	DeleteVariable(...) error

	// Settings (existing)
	UpdateServiceSettings(...) error
	UpdateServiceResources(...) error

	// Service CRUD (new)
	CreateService(ctx context.Context, name, icon string) (string, error)
	DeleteService(ctx context.Context, serviceID string) error

	// Domains (new)
	CreateCustomDomain(ctx context.Context, serviceID, domain string, port int) error
	DeleteCustomDomain(ctx context.Context, domainID string) error
	CreateServiceDomain(ctx context.Context, serviceID string, port int) error
	DeleteServiceDomain(ctx context.Context, domainID string) error

	// Volumes (new)
	CreateVolume(ctx context.Context, serviceID, name, mount, region string) error
	DeleteVolume(ctx context.Context, volumeID string) error

	// TCP Proxies (new)
	CreateTCPProxy(ctx context.Context, serviceID string, port int) error
	DeleteTCPProxy(ctx context.Context, proxyID string) error

	// Network (new)
	EnableNetwork(ctx context.Context, serviceID string) error
	DisableNetwork(ctx context.Context, endpointID string) error

	// Triggers (new)
	CreateTrigger(ctx context.Context, serviceID string, cfg TriggerConfig) error
	DeleteTrigger(ctx context.Context, triggerID string) error

	// Egress (new)
	SetEgress(ctx context.Context, serviceID string, regions []string) error

	// Deploy (new)
	TriggerDeploy(ctx context.Context, serviceID string) error
}
```

### Step 2: Implement in RailwayApplier

Each method delegates to the corresponding Railway client function.

### Step 3: Update Apply() to handle sub-resources

Add phases:

- Phase 1: Service CRUD (create new, delete marked)
- Phase 2: Service settings + resources
- Phase 3: Shared variables
- Phase 4: Per-service variables
- Phase 5: Sub-resources (domains, volumes, TCP proxies, network, triggers, egress)

### Step 4: Run tests — expect pass

### Step 5: Commit

---

## Task 15: Final verification

### Step 1: Regenerate all generated code

Run: `go generate ./internal/railway/`

### Step 2: Run `mise run check`

### Step 3: Run `go test -race ./...`
