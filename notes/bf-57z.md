# ccdash CI/CD GitHub Release Publishing (Bead bf-57z)

## Status: Already Implemented

The ccdash binary GitHub release publishing via Argo CI was already implemented in commits leading up to 2026-06-25.

## Implementation Summary

### 1. WorkflowTemplate: ccdash-ci
Location: /home/coding/declarative-config/k8s/iad-ci/argo-workflows/ccdash-ci.yaml
Deployed: 2026-05-27

Features:
- Multi-arch binary builds (linux-amd64, linux-arm64, darwin-amd64, darwin-arm64)
- SHA256 checksum generation (SHA256SUMS and individual .sha256 files per binary)
- GitHub release creation with all artifacts
- Tag-triggered releases
- Main branch CI checks (fmt, vet, build)

### 2. Sensors

#### ccdash-ci-sensor
Location: /home/coding/declarative-config/k8s/iad-ci/argo-events/ccdash-ci-sensor.yml
Purpose: Triggers on GitHub pushes to main branch for CI checks

#### ccdash-tag-trigger
Location: /home/coding/declarative-config/k8s/iad-ci/argo-events/ccdash-tag-trigger.yaml
Purpose: Triggers on Forgejo tag pushes (vX.Y.Z pattern) for release builds

### 3. EventSources

#### GitHub Webhooks
Location: /home/coding/declarative-config/k8s/iad-ci/argo-events/github-eventsource.yml
- Endpoint: /ccdash
- URL: https://webhooks-ci.ardenone.com/ccdash
- Events: push
- ✅ Configured on GitHub repo

#### Forgejo Webhooks
Location: /home/coding/declarative-config/k8s/iad-ci/argo-events/forgejo-eventsource.yml
- Endpoint: /ccdash
- URL: https://webhooks-ci.ardenone.com/ccdash
- ✅ Configured on Forgejo repo

## Release Process

To create a new release:

1. Tag the commit on Forgejo (source of truth):
   git tag v0.9.0
   git push origin v0.9.0

2. Automatic triggers:
   - Forgejo webhook receives tag push event
   - ccdash-tag-trigger sensor filters for refs/tags/v*.*.* pattern
   - Submits ccdash-ci workflow with tag parameters
   - Workflow builds all 4 architecture binaries
   - Generates SHA256 checksums
   - Creates GitHub release with all artifacts

3. Manual workflow launch (if needed):
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
           value: "v0.9.0"
         - name: version
           value: "0.9.0"
         - name: commit-sha
           value: "$(git rev-parse HEAD)"
   EOF

## Release Artifacts

Each release publishes:
- ccdash-linux-amd64
- ccdash-linux-arm64
- ccdash-darwin-amd64
- ccdash-darwin-arm64
- SHA256SUMS
- ccdash-*.sha256 (per-binary checksum files)

## Implementation Commits

- 1b4cc88 feat(ccdash-ci): add multi-arch build and GitHub release publishing
- 083c48c feat(ccdash-ci): finalize multi-arch GitHub release pipeline
- 4c6ee01 fix(ccdash-ci): update sensor and workflow templates
- eb4bc5a feat(ccdash-ci): add GitHub webhook eventsource configuration
- 6642395 fix(ccdash-ci): repair broken sensor by consolidating duplicate dependencies

## Verification

All components verified:
- ✅ WorkflowTemplate deployed and functioning
- ✅ Both sensors deployed and active
- ✅ GitHub webhook configured on repo
- ✅ Forgejo webhook configured on repo
- ✅ Multi-arch builds configured
- ✅ SHA256 checksums configured
- ✅ GitHub release creation configured
