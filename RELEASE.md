# Release Process

How to cut a new `ipfs-cluster` release.

## Versioning

Two files hold the version and must stay in sync. `release.sh` updates both.

- `version/version.go`
- `cmd/ipfs-cluster-ctl/main.go`

Tags are signed and follow `vX.Y.Z` (final) or `vX.Y.Z-rcN` (release candidate).

## Prerequisites

- Push access to `master`.
- A GPG key configured for signed commits and tags (`git commit -S`, `git tag -s`).
- Up-to-date local `master`: `git fetch origin && git checkout master && git pull --ff-only`.

## Steps

1. Update `CHANGELOG.md` with the new version section. Land it via PR from a `vX.Y.Z/changelog` branch and merge to `master`.
2. On `master`, run `./release.sh X.Y.Z`. The script bumps both version files, signs the release commit, and creates a signed annotated tag whose body is the commit log since the previous final tag.
3. Verify the tag locally: `git show vX.Y.Z`. Confirm the signature, the version bump, and the tag annotation.
4. Push the commit and the tag explicitly (do not use `git push --tags`):

   ```
   git push origin master
   git push origin vX.Y.Z
   ```

5. Wait for CI. The `docker-image` workflow builds multi-arch images and publishes `vX.Y.Z`, `stable`, and `latest` to Docker Hub (`ipfs/ipfs-cluster`).
6. Open a PR against [`ipfs/distributions`](https://github.com/ipfs/distributions) to publish the new binaries on `dist.ipfs.tech`. See prior PRs (e.g. `ipfs-cluster v1.1.5` #1188) as templates.

## Release candidates

Major releases typically go through one or more RCs (v1.0.0 had five). Patch releases usually skip them. To cut an RC, run `./release.sh X.Y.Z-rcN`. The script commits and creates a signed tag without a changelog body. Push the tag the same way:

```
git push origin master
git push origin vX.Y.Z-rcN
```

CI publishes only the `vX.Y.Z-rcN` Docker tag for release candidates. It does not move `stable` or `latest`. Open a separate `ipfs/distributions` PR for the RC.

## Notes

- `release.sh` runs `make clean` before editing files. A dirty working tree will be rebuilt from scratch.
- The annotated-tag body is generated from `git log <lastver>..HEAD`, so messy commits since the previous release surface in the tag and the GitHub Release page. Keep `master` tidy.
- Never use `git push --tags`. It pushes every local tag, including stale or experimental ones.
