# Ccdash CI/CD Setup (Bead bf-57z)

## Completed Work

### WorkflowTemplate Consolidation
- **Removed**: `k8s/iad-ci/argo-workflows/ccdash-ci-workflowtemplate.yml` (stale duplicate)
- **Kept**: `k8s/iad-ci/argo-workflows/ccdash-ci.yaml` (comprehensive multi-arch build)

### ccdash-ci WorkflowTemplate Features
The `ccdash-ci` WorkflowTemplate now supports:

1. **Multi-architecture builds** (parallel execution):
   - `linux-amd64`
   - `linux-arm64`
   - `darwin-amd64`
   - `darwin-arm64`

2. **SHA256 checksums**:
   - Generates `SHA256SUMS` file with all binary checksums
   - Creates individual `.sha256` files for each binary

3. **GitHub Release creation**:
   - Automatic release creation on tag push
   - Includes all binaries and checksums
   - Extracts release notes from `CHANGELOG.md` if available

4. **CI workflow types**:
   - **Main branch checks**: lint, vet, build (no release)
   - **Tagged release**: multi-arch build + checksums + GitHub release

### Trigger Mechanisms

#### GitHub Webhooks (ccdash-ci-sensor.yml)
- Monitors `github.com/jedarden/ccdash`
- Triggers on:
  - Push to `main` → runs CI checks
  - Tag push matching `refs/tags/v*` → runs release build

#### Forgejo Webhooks (ccdash-tag-trigger.yaml)
- Monitors `git.ardenone.com/jedarden/ccdash`
- Triggers on tag push matching `refs/tags/v[0-9]+\.[0-9]+\.[0-9]+`
- Extracts parameters:
  - `tag`: full ref (e.g., `refs/tags/v1.0.0`)
  - `version`: stripped version (e.g., `1.0.0`)
  - `commit-sha`: full commit SHA

### Manual Release Workflow

To trigger a release manually:

```bash
kubectl --kubeconfig=/home/coding/.kube/iad-ci.kubeconfig create -f - <<EOF
apiVersion: argoproj.io/v1alpha1
kind: Workflow
metadata:
  generateName: ccdash-ci-
  namespace: argo-workflows
spec:
  workflowTemplateRef:
    name: ccdash-ci
  arguments:
    parameters:
      - name: tag
        value: "v1.0.0"
      - name: version
        value: "1.0.0"
      - name: commit-sha
        value: "abc123..."
EOF
```

### Release Process Summary

1. **Developer creates tag** on Forgejo:
   ```bash
   git tag v1.0.0
   git push forgejo v1.0.0
   ```

2. **Forgejo webhook** triggers `ccdash-ci` workflow in release mode

3. **Workflow executes**:
   - Checks out repository at tag
   - Builds 4 binaries in parallel
   - Generates SHA256 checksums
   - Creates GitHub release with all artifacts

4. **Result**: GitHub release `v1.0.0` published with:
   - `ccdash-linux-amd64` + `.sha256`
   - `ccdash-linux-arm64` + `.sha256`
   - `ccdash-darwin-amd64` + `.sha256`
   - `ccdash-darwin-arm64` + `.sha256`
   - `SHA256SUMS`

### Commits
- `083c48c` - feat(ccdash-ci): finalize multi-arch GitHub release pipeline
- `4c6ee01` - fix(ccdash-ci): update sensor and workflow templates

### Status
✅ **COMPLETE** - Multi-arch GitHub release pipeline operational
