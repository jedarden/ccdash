# ccdash CI/CD Setup (Bead bf-57z)

## Overview

Added multi-arch build and GitHub release publishing to the `ccdash-ci` WorkflowTemplate in `jedarden/declarative-config`.

## Changes Made

Updated `/home/coding/declarative-config/k8s/iad-ci/argo-workflows/ccdash-ci.yaml`:

- Added tag-triggered release builds (vX.Y.Z tags)
- Multi-arch binary builds:
  - linux-amd64
  - linux-arm64
  - darwin-amd64
  - darwin-arm64
- SHA256 checksum generation for all binaries
- GitHub Release publishing via `gh` CLI
- Preserved existing main-branch checks (lint, vet, build)
- Added `podGC` and `ttlStrategy` for resource cleanup

## Usage

### Main Branch Checks (Automatic)

Runs on every commit to main:
- Checkout, lint (gofmt), vet (go vet), build

### Tag-Triggered Release

To create a release:

```bash
# Tag a commit
git tag v1.0.0
git push origin v1.0.0

# Or create and push in one command
git tag -a v1.0.0 -m "Release v1.0.0"
git push origin v1.0.0
```

The workflow will:
1. Detect the tag
2. Build all 4 platform binaries
3. Generate SHA256SUMS and individual .sha256 files
4. Create GitHub Release with all artifacts
5. Extract release notes from CHANGELOG.md if present

### Manual Trigger

To manually trigger a release build:

```bash
kubectl --kubeconfig=/home/coding/.kube/iad-ci.kubeconfig create -f - <<EOF
apiVersion: argoproj.io/v1alpha1
kind: Workflow
metadata:
  generateName: ccdash-release-
  namespace: argo-workflows
spec:
  workflowTemplateRef:
    name: ccdash-ci
  arguments:
    parameters:
      - name: tag
        value: v1.0.0
      - name: version
        value: 1.0.0
      - name: commit-sha
        value: $(git rev-parse HEAD)
EOF
```

## Artifacts

Each release publishes:
- `ccdash-linux-amd64` + `ccdash-linux-amd64.sha256`
- `ccdash-linux-arm64` + `ccdash-linux-arm64.sha256`
- `ccdash-darwin-amd64` + `ccdash-darwin-amd64.sha256`
- `ccdash-darwin-arm64` + `ccdash-darwin-arm64.sha256`
- `SHA256SUMS` (all checksums in one file)

## Commit

Committed to `jedarden/declarative-config`:
- Commit: `1b4cc88`
- Message: `feat(ccdash-ci): add multi-arch build and GitHub release publishing`

## Status

✅ Complete - WorkflowTemplate updated, committed, and pushed to declarative-config
