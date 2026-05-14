# CI Migration: GitHub Actions → Argo Workflows (bf-5c3)

## Summary

Completed CI migration from GitHub Actions to Argo Workflows. The `.github/workflows/release.yml`
was deleted in commit `958f114`. This task created and fixed the corresponding Argo WorkflowTemplate
in `jedarden/declarative-config`.

## What Was Done

1. **Deleted GitHub Actions workflow** (commit `958f114` in ccdash):
   - Removed `.github/workflows/release.yml`

2. **Created Argo WorkflowTemplate** (`jedarden/declarative-config`, commit `452a709`):
   - File: `k8s/iad-ci/argo-workflows/ccdash-ci-workflowtemplate.yml`
   - Builds multi-arch Go binaries: linux-amd64, linux-arm64, darwin-amd64, darwin-arm64
   - Runs `go test -v -race ./...` before building
   - Creates GitHub release with all 8 artifacts (binaries + SHA256 checksums)
   - Uses `github-webhook-secret` for GitHub API access

3. **Fixed bugs and cleaned up duplicates** (`jedarden/declarative-config`, commit `4759fdc`):
   - Removed duplicate `ccdash-ci.yaml` (was missing gh CLI installation — would fail)
   - Fixed VERSION extraction: `cat VERSION` (grep '^version' didn't match bare `0.9.4` format)
   - Fixed release tag: added `v` prefix (`v${VERSION}`)
   - Fixed `gh release create` paths after `cd bin` (now uses `./ccdash-*`)
   - Added `ccdash-ci-sensor.yml` (was untracked) — triggers on push to main

## ArgoCD Sync

The WorkflowTemplate and Sensor are synced to the `iad-ci` cluster via the
`argo-workflows-ns-iad-ci` and `argo-events-ns-iad-ci` ArgoCD applications.

## Usage

To trigger a release build manually:
```bash
kubectl --kubeconfig=/home/coding/.kube/iad-ci.kubeconfig create -f - <<EOF
apiVersion: argoproj.io/v1alpha1
kind: Workflow
metadata:
  generateName: ccdash-ci-manual-
  namespace: argo-workflows
spec:
  workflowTemplateRef:
    name: ccdash-ci
EOF
```

Automatic trigger fires on push to main when the `ccdash` event source receives a webhook.
The template is idempotent — it skips release creation if `v${VERSION}` already exists.
