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
- The checks run gofmt, `go vet`, `go test` and `go build`; a failing test
  blocks the tag, so run `go test ./...` before pushing.
- A push that changes only `.beads/`, `docs/`, `notes/` or Markdown files is
  checked but not released. Do not bump `VERSION` for such a push.
- If automatic tagging does not occur, investigate the workflow. After checks
  pass, create and push the expected tag on the verified release commit to
  trigger the release workflow. Never force-push or push to the GitHub mirror.
- If a CI run fails, fix the cause in the WorkflowTemplate (declarative-config)
  and let the next push carry the change; a version without a GitHub release
  loses nothing, because the next release has the same commits. Retrying a run
  by hand with a cluster write kubeconfig is break-glass only.

The workflow definitions live in `declarative-config` under
`k8s/iad-ci/argo-workflows/ccdash-ci.yaml` and the associated Argo Events
Sensors. Change their managed configuration through GitOps. How the pipeline
works is described in `docs/notes/release-pipeline.md`.

## Screenshots and demos

Never record real sessions, workspaces or costs for the README. Use
`scripts/demo-env.sh` (synthetic data on a private tmux server) and regenerate
the screenshot with `vhs docs/images/ccdash.tape`.
