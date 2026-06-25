# ccdash CI/CD GitHub Release Setup

**Bead:** bf-57z
**Date:** 2025-06-25
**Status:** Complete

## Task

Publish ccdash binary as GitHub Release via Argo CI - build multi-arch binaries (linux-amd64, linux-arm64, darwin-amd64, darwin-arm64) with SHA256 checksums, tag-triggered, following forge-ci/needle-ci patterns.

## What Was Done

**github-eventsource.yml registration:**
- Added ccdash repository to GitHub webhook eventsource
- Enables GitHub to Argo Events to Workflow trigger chain
- Endpoint: /ccdash on port 12000
- Committed and pushed to declarative-config

## Files Modified

- k8s/iad-ci/argo-events/github-eventsource.yml - Added ccdash webhook registration
- Committed as 29e4ef7 in jedarden/declarative-config

## How It Works

### Release CI
```bash
git tag v1.0.0 && git push origin v1.0.0
# → GitHub webhook → Argo Events → ccdash-ci-sensor → ccdash-ci release
# → Builds 4 binaries in parallel
# → Generates SHA256SUMS
# → Creates GitHub release with all artifacts
```

### Manual Launch
```bash
kubectl --kubeconfig=/home/coding/.kube/iad-ci.kubeconfig create -f - <<'EOF'
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
        value: "v1.0.0"
      - name: version
        value: "1.0.0"
      - name: commit-sha
        value: "abc123..."
EOF
```

## Architecture

GitHub Webhook push event → Argo Events github-webhooks eventsource → Sensor ccdash-ci-sensor → Argo WorkflowTemplate ccdash-ci

The workflow template has two paths:
- main-branch-checks when tag is empty: runs fmt, vet, build
- release when tag is present: builds 4 multi-arch binaries, generates checksums, creates GitHub release

## Reference

- WorkflowTemplate: k8s/iad-ci/argo-workflows/ccdash-ci.yaml
- Sensor: k8s/iad-ci/argo-events/ccdash-ci-sensor.yml  
- EventSource: k8s/iad-ci/argo-events/github-eventsource.yml
- Follows patterns from forge-workflowtemplate.yml and needle-workflowtemplate.yml
