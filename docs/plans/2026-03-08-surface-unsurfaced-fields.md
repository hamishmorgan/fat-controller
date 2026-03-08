# Surface Unsurfaced Fetched Fields

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make all fields fetched from Railway visible in `config get --full` output, and ensure user-settable fields are properly diffed and applied.

**Architecture:** Three categories of work: (1) read-only fields shown in `config get --full` only, (2) user-settable fields wired through diff + apply, (3) display gaps for fields already diffed/applied but not shown.

**Tech Stack:** Go, genqlient, TOML rendering, diff engine

---

## Category 1: Read-only fields (adopt-only in `config get --full`)

These fields are assigned by Railway and cannot be set by users. They should appear in `config get --full` output but not in diff or apply.

Fields: `LiveDomain.Suffix`, `LiveTCPProxy.ProxyPort`, `LiveTCPProxy.Domain`, `LiveEgressGateway.IPv4`, `LiveNetworkEndpoint.DNSName`

### Task 1: Carry sub-resource fields through maskConfig

**Files:**

- Modify: `internal/config/render.go:180-199` (`maskConfig`)
- Modify: `internal/config/render.go:315-333` (`envRefConfig`)

Currently `maskConfig` and `envRefConfig` create new `ServiceConfig` structs that only copy `ID`, `Name`, `Icon`, `Variables`, `Deploy`. They silently drop `Domains`, `Volumes`, `TCPProxies`, `Triggers`, `Egress`, `Network`, `VCPUs`, `MemoryGB`.

**Step 1: Update maskConfig to copy all fields**

In `maskConfig`, change the `ServiceConfig` construction to:

```go
out.Services[name] = &ServiceConfig{
    ID:         svc.ID,
    Name:       svc.Name,
    Icon:       svc.Icon,
    Variables:  maskVars(svc.Variables, masker),
    Deploy:     svc.Deploy,
    VCPUs:      svc.VCPUs,
    MemoryGB:   svc.MemoryGB,
    Domains:    svc.Domains,
    Volumes:    svc.Volumes,
    TCPProxies: svc.TCPProxies,
    Triggers:   svc.Triggers,
    Egress:     svc.Egress,
    Network:    svc.Network,
}
```

Do the same for `envRefConfig`.

**Step 2: Run tests**

Run: `go test ./internal/config/...`

**Step 3: Commit**

```text
fix: carry sub-resource fields through maskConfig and envRefConfig
```

### Task 2: Add sub-resources to TOML show output

**Files:**

- Modify: `internal/config/render.go` (tomlService struct, tomlDeploy struct, liveToTOMLServices, deployToTOML)

**Step 1: Add missing deploy fields to tomlDeploy**

Add to `tomlDeploy`:

```go
WatchPatterns    []string `toml:"watch_patterns,omitempty"`
PreDeployCommand []string `toml:"pre_deploy_command,omitempty"`
```

Update `deployToTOML` to copy them:

```go
td.WatchPatterns = d.WatchPatterns
td.PreDeployCommand = d.PreDeployCommand
```

Update the zero-check to include them:

```go
&& td.WatchPatterns == nil && td.PreDeployCommand == nil
```

**Step 2: Add sub-resource and resource fields to tomlService**

Add to `tomlService`:

```go
Resources  *tomlResources          `toml:"resources,omitempty"`
Domains    map[string]tomlDomain   `toml:"domains,omitempty"`
Volumes    map[string]tomlVolume   `toml:"volumes,omitempty"`
TCPProxies []int                   `toml:"tcp_proxies,omitempty"`
Network    *bool                   `toml:"network,omitempty"`
Triggers   []tomlTrigger           `toml:"triggers,omitempty"`
Egress     []string                `toml:"egress,omitempty"`
```

Add new TOML render structs:

```go
type tomlResources struct {
    VCPUs    *float64 `toml:"vcpus,omitempty"`
    MemoryGB *float64 `toml:"memory_gb,omitempty"`
}

type tomlDomain struct {
    Port   *int   `toml:"port,omitempty"`
    Suffix string `toml:"suffix,omitempty"` // read-only, service domains only
}

type tomlVolume struct {
    Mount  string `toml:"mount"`
    Region string `toml:"region,omitempty"`
}

type tomlTrigger struct {
    Repository string `toml:"repository"`
    Branch     string `toml:"branch"`
    Provider   string `toml:"provider,omitempty"` // read-only
}
```

**Step 3: Update liveToTOMLServices to populate new fields (gated behind `full`)**

In `liveToTOMLServices`, inside the `if full` block, add logic to populate:

- `ts.Resources` from `svc.VCPUs` / `svc.MemoryGB`
- `ts.Domains` from `svc.Domains` (map by domain name, include Port and Suffix)
- `ts.Volumes` from `svc.Volumes` (map by volume name, include Mount and Region)
- `ts.TCPProxies` from `svc.TCPProxies` (list of ApplicationPort values)
- `ts.Network` from `svc.Network` (true if non-nil, omit if nil)
- `ts.Triggers` from `svc.Triggers` (list of repo+branch+provider)
- `ts.Egress` from `svc.Egress` (list of regions)

**Step 4: Run tests, commit**

### Task 3: Add sub-resources to JSON show output

**Files:**

- Modify: `internal/config/render.go` (`toJSONMap`, `deployMap`)

**Step 1: Add missing fields to deployMap**

Add to `deployMap`:

```go
if len(d.WatchPatterns) > 0 {
    m["watch_patterns"] = d.WatchPatterns
}
if len(d.PreDeployCommand) > 0 {
    m["pre_deploy_command"] = d.PreDeployCommand
}
```

**Step 2: Add sub-resources and resources to toJSONMap**

In the `if full` block for each service, add:

```go
if svc.VCPUs != nil || svc.MemoryGB != nil {
    res := map[string]any{}
    if svc.VCPUs != nil { res["vcpus"] = *svc.VCPUs }
    if svc.MemoryGB != nil { res["memory_gb"] = *svc.MemoryGB }
    svcMap["resources"] = res
}
// domains, volumes, tcp_proxies, network, triggers, egress...
```

**Step 3: Run tests, commit**

### Task 4: Add sub-resources to text show output

**Files:**

- Modify: `internal/config/render.go` (`renderText`)

In the `if full` block for each service, after deploy settings, render:

- `[service.resources]` with vcpus/memory_gb
- `[service.domains]` table
- `[service.volumes]` table
- `tcp_proxies = [...]`
- `network = true/false`
- `triggers = [...]`
- `egress = [...]`

**Step 1: Implement, Step 2: Run tests, Step 3: Commit**

---

## Category 2: User-settable fields (wire through diff + apply)

### Task 5: Wire volume region through diff and apply

**Files:**

- Modify: `internal/diff/diff.go` (`diffVolumes`) — pass region to SubResourceChange
- Modify: `internal/diff/diff.go` (`SubResourceChange`) — add Region string field
- Modify: `internal/apply/apply.go` (`applyVolumeChange`, `Applier` interface) — pass region
- Modify: `internal/apply/railway.go` (`CreateVolume`) — pass region to railway.CreateVolume
- Modify: `internal/railway/volumes.go` (`CreateVolume`) — accept region parameter

Currently `diffVolumes` only checks name existence (create/delete). It should also use `VolumeConfig.Region` in the create change, and `CreateVolume` should pass region to the Railway API.

**Step 1: Add Region to SubResourceChange** (already partially there — it has `Regions []string` for egress, add `Region string` for single-region resources)

Wait — `SubResourceChange` doesn't have a single `Region` field. Check if we can reuse `Regions` or add one. The cleanest approach: add `Region string` to `SubResourceChange`.

**Step 2: Update diffVolumes to include Region in create changes**

```go
changes = append(changes, SubResourceChange{
    Type:   "volume",
    Action: ActionCreate,
    Key:    volName,
    Mount:  vc.Mount,
    Region: vc.Region,  // <-- add this
})
```

**Step 3: Update Applier.CreateVolume signature**

```go
CreateVolume(ctx context.Context, serviceID, mountPath, region string) error
```

**Step 4: Update applyVolumeChange to pass region**

```go
return applier.CreateVolume(ctx, serviceID, ch.Mount, ch.Region)
```

**Step 5: Update RailwayApplier.CreateVolume and railway.CreateVolume**

The Railway API's `VolumeCreateInput` likely has a region field — check schema.

**Step 6: Update mock applier in tests, run tests, commit**

### Task 6: Wire trigger provider through diff and apply

**Files:**

- Modify: `internal/diff/diff.go` (`diffTriggers`, `SubResourceChange`) — add Provider field
- Modify: `internal/apply/apply.go` (`applyTriggerChange`, `Applier` interface) — pass provider
- Modify: `internal/apply/railway.go` (`CreateDeploymentTrigger`) — pass provider
- Modify: `internal/railway/triggers.go` (`CreateDeploymentTrigger`) — accept provider param

**Step 1: Add Provider to SubResourceChange**

```go
Provider string // for triggers
```

**Step 2: Update diffTriggers to include Provider**

```go
changes = append(changes, SubResourceChange{
    Type:     "trigger",
    Action:   ActionCreate,
    Key:      tc.Repository + ":" + tc.Branch,
    Repo:     tc.Repository,
    Branch:   tc.Branch,
    Provider: tc.Provider,  // <-- add this
})
```

**Step 3: Update Applier.CreateDeploymentTrigger signature**

```go
CreateDeploymentTrigger(ctx context.Context, serviceID, repo, branch, provider string) error
```

**Step 4: Update applyTriggerChange, RailwayApplier, railway.CreateDeploymentTrigger**

**Step 5: Update mock applier in tests, run tests, commit**

---

## Category 3: Display gaps (already diffed/applied but not shown)

These are covered by Tasks 2-4 above (WatchPatterns, PreDeployCommand, VCPUs, MemoryGB in deploy/resources rendering).

---

## Category 4: Remove dead field

### Task 7: Remove LiveVolume.ID

**Files:**

- Modify: `internal/config/model.go` — remove `ID` from `LiveVolume`
- Modify: `internal/railway/state.go` — stop setting `ID` on volume instances

`LiveVolume.ID` is the volume instance ID, but `VolumeID` (the volume entity ID) is what's used for deletes. The instance ID is never referenced.

**Step 1: Remove field, fix compilation, run tests, commit**

---

## Verification

After all tasks:

```bash
go generate ./internal/railway/...
go build ./...
go test ./...
mise check
```
