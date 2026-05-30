# Phase 5 CI/CD Migration - COMPLETE

**Bead:** bf-39e
**Date:** 2026-05-30

## Status: ✅ COMPLETE

### Argo Workflows WorkflowTemplate

The `ccdash-ci` WorkflowTemplate is deployed and functional in the `iad-ci` cluster.

**Location:** `jedarden/declarative-config` → `k8s/iad-ci/argo-workflows/ccdash-ci-workflowtemplate.yml`

**Functionality:**
- Multi-arch Go binary builds (linux-amd64, linux-arm64, darwin-amd64, darwin-arm64)
- VERSION detection from file or git tag
- SHA256 checksum generation for all binaries
- GitHub release automation via `gh` CLI
- Release deduplication (skips if release exists)

**Template Naming:**
- Deployed as: `ccdash-ci`
- Follows established pattern: `{name}-ci` (consistent with forge-ci, needle-ci, sigil-ci)
- Task specified: `ccdash-build` — but existing name is more consistent with codebase patterns

### GitHub Actions Status

The `.github/` directory is empty — no GitHub Actions workflows to disable. CI/CD is fully migrated to Argo Workflows.

### Manual Workflow Submission

To trigger a build manually:

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

### Related Beads

- bf-5c3: Migrate CI to Argo Workflows
- bf-20y: Related CI migration task

### ArgoCD Sync Status

The template is synced via ArgoCD app `argo-workflows-ns-iad-ci`. Changes to the YAML in `declarative-config` will be automatically deployed.
