# CCDash Repository Guide

The workspace guidance in `/home/coding/AGENTS.md` also applies here.

## Shipping changes

- When a change is ready to ship, update `VERSION` and `CHANGELOG.md`. Use a
  version explicitly requested for the release; otherwise increment the patch
  component of the latest release tag.
- Commit the change on `main` and push to the Forgejo `origin`. Follow the
  `ccdash-push-trigger` Argo workflow through its checks and tag creation. The
  resulting tag push runs `ccdash-tag-trigger` and the release workflow.
- Verify that the expected tag points at the release commit, the Argo checks
  and release build succeeded, and the GitHub release has its binaries and
  checksums. A local version bump or Git push alone does not finish a release.
- If automatic tagging does not occur, investigate the workflow. After checks
  pass, create and push the expected tag on the verified release commit to
  trigger the release workflow. Never force-push or push to the GitHub mirror.

The workflow definitions live in `declarative-config` under
`k8s/iad-ci/argo-workflows/ccdash-ci.yaml` and the associated Argo Events
Sensors. Change their managed configuration through GitOps.
