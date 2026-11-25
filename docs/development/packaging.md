# Packaging poomsae

## Release ownership

Releases happen externally. This repository keeps build and packaging tools
and checks snapshot builds in pull requests. Pushing a tag here does not
publish a release. Release operators should use the external release process.

## Container image

Poomsae defines its image; the external release process builds and publishes it as
`ghcr.io/ifigueroap/poomsae:<version>`. `docker-bake.hcl` declares an `image` target
for local builds and `oci-amd64` and `oci-arm64` targets that write one OCI
archive per platform to `OCI_OUTPUT_DIR`. The targets read `VERSION`,
`OCI_VERSION`, `COMMIT`, `REVISION`, and `BUILD_DATE` for the binary version
and image labels. GoReleaser does not build images.

`docker/release/Dockerfile` builds the web bundle and a static `poomsae` binary
from source, then runs it directly as UID/GID `1000` with `/data` as the Poomsae
home. Runtime setup belongs to Poomsae's env/flag interface; the image has no
entrypoint wrapper or config seed.

To build a local image:

```sh
docker buildx bake image --load --set image.tags=poomsae:local
```

The workflow-validation check builds both platform archives, loads the native
image, and exercises its version, user, embedded web UI, env-only foreground
startup, health probes, token permissions, and scheduled backups without
publishing.

See [containers](../operations/containers.md) for operator setup.

## Building packages

Package managers must identify themselves in the Poomsae binary so `poomsae update`
does not replace a file owned by the package database. Set the version,
distribution, and build date explicitly when building from a release source
archive:

```sh
go build -trimpath -buildvcs=false \
  -ldflags="-X github.com/ifigueroap/poomsae/internal/version.Version=v0.14.2 \
  -X github.com/ifigueroap/poomsae/internal/version.Distribution=example-pkg \
  -X github.com/ifigueroap/poomsae/internal/version.BuildDate=2026-08-08T00:00:00Z" \
  ./cmd/poomsae
```

Release source archives do not contain `.git`, so the VCS fallback cannot
derive a version. Without the `Version` linker value, `poomsae version` reports
`dev`. Use a literal release-commit timestamp for `BuildDate` so rebuilds and
architecture-specific packages report the same value. `Commit` may remain
`unknown` when building with `-buildvcs=false`.

From a clone, `make homebrew` runs the normal `make build` steps with cgo
enabled and `Distribution=homebrew`. `VERSION`, `COMMIT`, `BUILD_DATE`, and
`BINDIR` (default `.`) can be set on the command line:

```sh
make homebrew VERSION=v0.14.2 BUILD_DATE=2026-08-08T00:00:00Z BINDIR=/path/to/bin
```

Other package managers can set their own value with
`make build DISTRIBUTION=example-pkg`. The default empty `DISTRIBUTION` keeps
`poomsae update` enabled.

## Distribution values

Poomsae recognizes these package-owner values:

| Value | Owner | Upgrade path |
| --- | --- | --- |
| empty | Source build or ordinary Poomsae release archive | `poomsae update` |
| `kenn` | Official direct-download Poomsae build | `poomsae update` |
| `homebrew` | Official Homebrew tap or Homebrew Core | `brew upgrade poomsae` |
| `deb` | Poomsae `.deb` package | Install the newer `.deb` with the system package manager |
| `rpm` | Poomsae `.rpm` package | Install the newer `.rpm` with the system package manager |

Third-party packagers should set a stable, recognizable non-empty value such
as `nixpkgs` or `arch`. Any value other than empty or `kenn` makes
install-capable forms of `poomsae update` fail with a usage error before checking
the network or writing a file. `poomsae update --check` remains available and reports the owning
distribution plus package-manager guidance. `poomsae version --json` exposes the
value as `distribution` for support diagnostics.

This boundary is required because a self-updating binary inside a managed
prefix would bypass package checksums and fight the package manager over file
ownership. Poomsae deliberately does not infer ownership from its executable
path.

## Browser assets

The repository keeps a compiling development stub under `internal/web/dist`.
A release package must replace it with the production web bundle before the Go
build. Use Poomsae's dedicated release source archive, which already contains
those generated assets, or run the pinned frontend build from a clone:

```sh
make web-install web-embed
go run ./cmd/poomsae _web-assets-check
```

The hidden `_web-assets-check` command is suitable for package tests. Poomsae's
dedicated source archive includes vendored Go dependencies as well as the
production assets, so package builds need neither Node/Bun nor network access.
Build that archive with `-mod=vendor`; release verification enforces this path
with an empty module cache and `GOPROXY=off`.

## Official tap publishing

Release publication happens externally and does not write to
`kenn-io/homebrew-tap`. The tap's hourly workflow reads Poomsae's latest stable
release, validates it, and opens a same-repository formula update pull request
with the tap repository's short-lived `GITHUB_TOKEN`.

The release repository and tap updater share this exact archive-name contract:

```text
poomsae_<version>_homebrew_darwin_amd64.tar.gz
poomsae_<version>_homebrew_darwin_arm64.tar.gz
poomsae_<version>_homebrew_linux_amd64.tar.gz
poomsae_<version>_homebrew_linux_arm64.tar.gz
```

Renaming any of these archives requires a coordinated tap updater change before
the release. A missing or renamed archive fails the scheduled tap workflow;
the red workflow run and GitHub's failed scheduled-workflow notification are
the maintainer alert for a broken cross-repository contract.
