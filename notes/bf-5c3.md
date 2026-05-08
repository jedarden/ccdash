# CI Migration: GitHub Actions → Argo Workflows (bf-5c3)

## Summary

Completed CI migration from GitHub Actions to Argo Workflows. The `.github/workflows/release.yml` was previously deleted in commit `958f114` but the corresponding Argo WorkflowTemplate was never created. This task completes that migration.

## What Was Done

1. **Created Argo WorkflowTemplate** in `jedarden/declarative-config`:
   - File: `k8s/iad-ci/argo-workflows/ccdash-ci.yaml`
   - Builds multi-arch Go binaries: linux-amd64, linux-arm64, darwin-amd64, darwin-arm64
   - Runs tests before building
   - Creates GitHub release with all binaries and SHA256 checksums
   - Uses `github-webhook-secret` for GitHub API access
   - Reads version from `VERSION` file

2. **Committed and pushed** to declarative-config (commit `452a709`)

## ArgoCD Sync

The workflow template will be automatically synced to the `iad-ci` cluster via the `argo-workflows-ns-iad-ci` ArgoCD application.

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
