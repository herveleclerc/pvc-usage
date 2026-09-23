# kubectl-pvc-usage

`kubectl-pvc-usage` is a `kubectl` plugin written in Go that detects unused `PersistentVolumeClaims` (PVCs), calculates how long they have been idle, and helps identify orphaned storage volumes to optimize cloud costs.

It natively leverages the Kubernetes 1.37 Beta feature **`PersistentVolumeClaimUnusedSinceTime`** ([KEP-5541](https://www.kubernetes.dev/resources/keps/5541/)), which automatically populates an `Unused` condition on PVCs managed by the PVC protection controller.

---

## Technical Article

A complete technical deep dive and walkthrough is available in:
[Billet technique: Disques fantomes et factures astronomiques (Kubernetes 1.37 & kubectl plugins)](blog/billet-technique-k8s-1.37-plugin-kubectl.md)

---

## Features

- **Native Kubernetes 1.37 Integration**: Directly reads `.status.conditions[type="Unused"]` managed by `pvc-protection-controller`.
- **Idle Duration Calculation**: Converts `lastTransitionTime` into compact human-readable durations (e.g. `35d 4h`, `2h 15m`).
- **Extended Age Filters**: Supports filtering by idle age using days and hours (`--min-age=30d`, `--min-age=7d`, `--min-age=24h`).
- **Pod Cross-Referencing**: Identifies active pods referencing each PVC (`--show-pods`).
- **Multiple Output Formats**: Supports `table`, `wide`, `json`, and `yaml`.
- **FinOps Storage Summary**: Summarizes total examined PVCs, idle volume counts, and orphaned storage capacity.
- **Graceful Backward Compatibility**: Identifies clusters prior to Kubernetes 1.37 and reports missing condition status without crashing.
- **Multi-Architecture**: Compiles cleanly for `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`, and `windows/amd64`.

---

## Installation

### From Pre-compiled Binaries

Download the appropriate binary for your OS and architecture from the GitHub Releases page:

```bash
# Example for macOS (Apple Silicon / ARM64)
curl -sSL -o kubectl-pvc-usage https://github.com/herveleclerc/pvc-usage/releases/download/v0.1.0/kubectl-pvc-usage_darwin_arm64.tar.gz
tar -xzf kubectl-pvc-usage_darwin_arm64.tar.gz
chmod +x kubectl-pvc-usage
sudo mv kubectl-pvc-usage /usr/local/bin/
```

### Via Krew

```bash
kubectl krew install --manifest=krew.yaml
```

### From Source

```bash
git clone https://github.com/herveleclerc/pvc-usage.git
cd pvc-usage
make install
```

The binary will be built and copied to `$GOPATH/bin/kubectl-pvc-usage`.

---

## Usage

Once installed in your `$PATH`, the plugin can be invoked either as `kubectl pvc-usage` or directly as `kubectl-pvc-usage`.

### 1. Inspect PVCs in the current namespace

```bash
kubectl pvc-usage
```

Example output:

```text
NAMESPACE       NAME       STATUS  UNUSED-SINCE  CAPACITY  STORAGECLASS  AGE
production      pg-backup  Unused  42d 8h        200Gi     gp3-encrypted 90d
production      app-data   InUse   Active        50Gi      gp3-encrypted 15d

--- Storage Usage Summary (FinOps) ---
Total PVCs examined:   2 (Total capacity: 250Gi)
Unused PVCs:           1 (Orphaned capacity: 200Gi)
Active in-use PVCs:    1
```

### 2. Find all unused PVCs across the entire cluster

```bash
kubectl pvc-usage -A --unused-only
```

### 3. Identify volumes idle for more than 30 days (sorted by idle time)

```bash
kubectl pvc-usage -A --unused-only --min-age=30d --sort-by=unused
```

### 4. Detailed view with volume names and referencing pods

```bash
kubectl pvc-usage -n staging -o wide --show-pods
```

Example output:

```text
NAMESPACE  NAME       STATUS  UNUSED-SINCE  CAPACITY  STORAGECLASS  VOLUME                                    ACCESSMODES    PODS      AGE  REASON
staging    test-data  InUse   Active        1Gi       standard      pvc-7bd3964b-3e0a-4478-9a00-8e460027273b  ReadWriteOnce  test-app  45s  PodUsingPVC

--- Storage Usage Summary (FinOps) ---
Total PVCs examined:   1 (Total capacity: 1Gi)
Unused PVCs:           0 (Orphaned capacity: 0)
Active in-use PVCs:    1
```

### 5. Export to JSON or YAML for automated pipelines

```bash
kubectl pvc-usage -A --unused-only -o json
```

---

## Available Flags

```text
Flags:
  -A, --all-namespaces         List across all namespaces
  -u, --unused-only            Only display PVCs currently marked as unused
      --in-use-only            Only display PVCs currently referenced by pods
      --min-age string         Filter PVCs unused for at least this duration (e.g. '30d', '7d', '24h')
      --storage-class string   Filter by StorageClass name
      --sort-by string         Sort results by: 'namespace', 'name', 'unused', 'size', 'age' (default: "namespace")
      --show-pods              Cross-reference pods in namespaces to list referencing pods
  -o, --output string          Output format: 'table', 'wide', 'json', 'yaml' (default: "table")
      --summary                Display FinOps storage summary at end of table output (default: true)
      --no-headers             Do not print table header

Standard kubectl flags inherited via cli-runtime:
      --kubeconfig string      Path to the kubeconfig file to use for CLI requests
      --context string         The name of the kubeconfig context to use
  -n, --namespace string       If present, the namespace scope for this CLI request
      --as string              Username to impersonate for the operation
```

---

## Building and Testing

### Run Unit Tests

```bash
make test
```

### Multi-Architecture Build

```bash
make build-all
```

Binaries will be placed in `./dist`:
- `kubectl-pvc-usage-linux-amd64`
- `kubectl-pvc-usage-linux-arm64`
- `kubectl-pvc-usage-darwin-amd64`
- `kubectl-pvc-usage-darwin-arm64`
- `kubectl-pvc-usage-windows-amd64.exe`

---

## License

Apache License 2.0. See LICENSE for details.
