# CI Migration: GitHub Actions → Argo Workflows

## Status: Complete

Migration completed in commit `958f114`.

## Changes Made

### 1. Removed GitHub Actions
- Deleted `.github/workflows/release.yml`
- CI no longer runs on GitHub Actions

### 2. Added Argo WorkflowTemplate
- Created `ccdash-ci-workflowtemplate.yml` in declarative-config
- Template builds multi-platform binaries (linux/amd64, linux/arm64, darwin/amd64, darwin/arm64)
- Runs tests with `go test -v -race ./...`
- Creates GitHub releases with `gh` CLI

### 3. Workflow Behavior
- Clones repo from GitHub
- Reads version from VERSION file or git describe
- Builds 4 platform binaries with SHA256 checksums
- Creates GitHub release if it doesn't already exist

## Infrastructure Notes

The Argo WorkflowTemplate exists in declarative-config but is not currently deployed to the cluster due to an ArgoCD configuration issue:
- The `argo-workflows-ns-iad-ci` application references an invalid cluster URL
- Cluster `https://hcp-de5bec10-ce14-4eed-a6f4-750f3fd3a89a.spot.rackspace.com` not found in ArgoCD
- This is a separate infrastructure issue that needs to be resolved

## Manual Workflow Submission

Until ArgoCD sync is fixed, workflows can be submitted manually:

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
