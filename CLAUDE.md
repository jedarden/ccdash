# CCDash Release Instructions

Follow [AGENTS.md](AGENTS.md) and the workspace guide at
`/home/coding/AGENTS.md` for repository work.

For every release, use the version explicitly requested or bump the patch
version from the latest tag. Update `VERSION` and `CHANGELOG.md`, commit and
push `main` to Forgejo `origin`, then follow the Argo CI/CD workflow through
tagging, build, and published release assets. Do not stop at the local commit
or tag.
