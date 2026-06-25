# bf-57z: ccdash GitHub Release via Argo CI

## Task
Publish ccdash binary as GitHub Release via Argo Workflows CI on tag push.

## Implementation

Complete CI/CD setup for ccdash including Forgejo repository creation, webhook configuration, and multi-arch binary release automation.

### Components

1. **WorkflowTemplate: ccdash-ci** (declarative-config/k8s/iad-ci/argo-workflows/ccdash-ci.yaml)
   - Main branch: runs lint, vet, build checks
   - Tag push (vX.Y.Z): builds multi-arch binaries and publishes GitHub Release
   - Multi-arch targets: linux-amd64, linux-arm64, darwin-amd64, darwin-arm64
   - SHA256 checksums: generates SHA256SUMS + individual .sha256 files for each binary
   - Parallel build matrix: all 4 targets build concurrently
   - **Status**: ✅ Deployed to argo-workflows namespace

2. **Sensor: ccdash-tag-trigger** (declarative-config/k8s/iad-ci/argo-events/ccdash-tag-trigger.yaml)
   - Filters Forgejo webhook push events for refs/tags/v* pattern
   - Extracts tag, version, commit-sha from payload
   - Submits ccdash-ci workflow with release parameters
   - **Status**: ✅ Deployed to argo-events namespace

3. **EventSource: forgejo-webhooks** (declarative-config/k8s/iad-ci/argo-events/forgejo-eventsource.yml)
   - ccdash webhook endpoint at /ccdash on port 12000
   - Receives webhooks from Forgejo via Tailscale ingress
   - **Status**: ✅ Configured and deployed

4. **Forgejo Repository** (https://git.ardenone.com/jedarden/ccdash)
   - **Status**: ✅ Created and code pushed
   - **Webhook**: ✅ Configured pointing to https://webhooks-ci.ardenone.com/ccdash
   - **Git Remote**: ✅ Updated (Forgejo as origin, GitHub as backup)

### Pipeline Flow

Tag push (git tag v1.0.0 && git push origin v1.0.0)
  → Forgejo webhook
  → Argo Events eventsource (traefik-iad-ci:12000/ccdash)
  → Sensor filters for tags
  → Submits ccdash-ci workflow
  → Builds 4 multi-arch binaries in parallel
  → Generates SHA256 checksums
  → Creates GitHub Release with all artifacts

### Changes Made

1. **Forgejo Repository Setup**
   - Created jedarden/ccdash repo on Forgejo
   - Pushed existing code from GitHub to Forgejo
   - Updated git remote: origin → Forgejo, github → GitHub (backup)

2. **Forgejo Webhook Configuration**
   - Created webhook on Forgejo repo
   - URL: https://webhooks-ci.ardenone.com/ccdash (updated 2026-06-25)
   - Events: push
   - Content type: json

3. **Git Remote Configuration**
   - origin: https://git.ardenone.com/jedarden/ccdash.git (primary)
   - github: https://github.com/jedarden/ccdash.git (mirror/backup)

### Manual Trigger

kubectl --kubeconfig=/home/coding/.kube/iad-ci.kubeconfig create -f - <<'MANUAL'
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
        value: "v0.1.0"
      - name: version
        value: "0.1.0"
      - name: commit-sha
        value: "<commit-sha>"
MANUAL

### Remaining Manual Step

**GitHub Mirror Setup**: Configure server-side push mirror from Forgejo to GitHub via Forgejo UI (requires admin permissions). Navigate to:
https://git.ardenone.com/jedarden/ccdash/settings/mirrors

## Status
✅ Complete - CI/CD pipeline fully operational with Forgejo as source of truth
