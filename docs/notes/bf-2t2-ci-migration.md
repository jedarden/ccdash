# CI Migration to Argo Workflows (bf-2t2)

Date: 2025-01-09

## Summary
Migrated ccdash CI from GitHub Actions to Argo Workflows on iad-ci cluster.

## Changes
- Created `k8s/iad-ci/argo-workflows/ccdash-ci.yaml` in jedarden/declarative-config
- CI workflow includes:
  - `go fmt` check (ensures code formatting)
  - `go vet` (static analysis)
  - `go build` (build verification)

## Workflow Template
- Name: `ccdash-ci`
- Service Account: `argo-workflow`
- Parameters:
  - `branch`: defaults to `main`
  - `repo`: defaults to `https://github.com/jedarden/ccdash.git`
- Steps: checkout → (lint, vet, build run in parallel)

## Trigger
The workflow can be triggered via:
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

## Commit
- declarative-config: `4ef8bd1` - feat: add ccdash CI workflow template
