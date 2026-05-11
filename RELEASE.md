# Release Process

## Versioning

`release.sh` keeps two files in sync:

- `version/version.go`
- `cmd/ipfs-cluster-ctl/main.go`

Tags are signed: `vX.Y.Z` for finals, `vX.Y.Z-rcN` for release candidates.

## Prerequisites

- Push access to `master`.
- A GPG key for signing commits and tags.
- Local `master` up to date.

## Steps

1. Land a `CHANGELOG.md` PR from a `vX.Y.Z/changelog` branch.
2. On `master`, run `./release.sh X.Y.Z`. The script bumps both version files, signs the release commit, and creates a signed annotated tag.
3. Run `git show vX.Y.Z` and confirm the GPG signature and tag annotation look right.
4. Push commit and tag explicitly (never `git push --tags`):

   ```
   git push origin master
   git push origin vX.Y.Z
   ```

5. Wait for the `docker-image` workflow to publish `vX.Y.Z`, `stable`, and `latest` to [Docker Hub](https://hub.docker.com/r/ipfs/ipfs-cluster/tags).
6. Create a GitHub Release for the tag (web UI or `gh release create vX.Y.Z`) and paste the new `CHANGELOG.md` entry as the body.
7. Close the `Release vX.Y.Z` milestone and open `Release vX.Y.(Z+1)`.
8. Open a PR against [`ipfs/distributions`](https://github.com/ipfs/distributions/) to publish binaries on `dist.ipfs.tech` (see [ipfs/distributions#1188](https://github.com/ipfs/distributions/pull/1188) as a template).

## Release candidates

Major releases usually go through one or more RCs (v1.0.0 had five). Patch releases skip them. Run `./release.sh X.Y.Z-rcN`, then push the commit and tag the same way. CI publishes only the `vX.Y.Z-rcN` tag to [Docker Hub](https://hub.docker.com/r/ipfs/ipfs-cluster/tags) and does not move `stable` or `latest`. Open a separate [`ipfs/distributions`](https://github.com/ipfs/distributions/) PR for the RC.

## Notes

- `release.sh` runs `make clean` before editing files.
- The tag annotation comes from `git log <lastver>..HEAD`, so messy commits surface on the GitHub Release page. Keep `master` tidy.
- Never `git push --tags`: it pushes every local tag, including stale ones. Push the branch and the new tag explicitly: `git push origin master` followed by `git push origin vX.Y.Z`.
