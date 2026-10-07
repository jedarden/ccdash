# Release pipeline

How a change becomes a GitHub release. The pipeline is the `ccdash-ci`
WorkflowTemplate in `jedarden/declarative-config`
(`k8s/iad-ci/argo-workflows/ccdash-ci.yaml`), synced to the iad-ci cluster by
the ArgoCD application `argo-workflows-ns-iad-ci`. Change it there, not here:
this repository keeps no copy of it.

## What happens on a push

1. **Push to `main`** (Forgejo). The `ccdash-push-trigger` sensor starts the
   `auto-version` entrypoint for the pushed commit.
2. **Checks**, in one pod: `gofmt`, `go vet`, `go test -p 2 ./...` (with `jq`
   installed, because the hook tests run the real hook scripts) and `go build`.
   A failure stops here, so no tag is created.
3. **Release decision.** If nothing outside `.beads/`, `docs/`, `notes/` or
   Markdown files changed since the latest tag, the run ends without
   releasing. Otherwise it uses `VERSION` when the pushed commits changed it,
   or bumps the patch component itself, then tags.
4. **Tag push.** The `ccdash-tag-trigger` sensor starts the `release`
   entrypoint: one pod clones the tag, cross-compiles linux/darwin x
   amd64/arm64, writes `SHA256SUMS` and per-binary `.sha256` files, and
   publishes the GitHub release with that version's `CHANGELOG.md` section as
   notes.

A push takes about 2-3 minutes to tag; a release about 4-5 minutes more.

## When something fails

Each pipeline is a single pod with a pod-local volume, so there are no shared
volumes to attach. Pod-level errors are retried (`retryPolicy: OnError`), and
publishing retries transient GitHub API errors because it is idempotent: an
existing release gets its assets re-uploaded.

If a run still fails, fix the cause in the template and let the next push
carry the change. A version that never got a GitHub release loses nothing,
because the next release contains the same commits. Retrying a run by hand
with a cluster write kubeconfig is break-glass only.

## Shipping a change

Follow [`../../AGENTS.md`](../../AGENTS.md): set `VERSION`, add the
`CHANGELOG.md` section, push to Forgejo `origin`, and confirm the tag and the
GitHub release (nine assets).
