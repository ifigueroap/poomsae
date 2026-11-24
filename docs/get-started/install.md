# Install

poomsae ships as a single Go binary. It stores data locally in SQLite and has no
runtime service dependency beyond the daemon it starts itself.

## Requirements

The release installers below do not require Go. If you use `go install` or
build from a clone, install Go 1.27.4 or later from <https://go.dev/dl/>.

GitHub release binaries are available starting with `v0.5.0`. Since v0.14.0,
poomsae publishes stable releases that preserve backward compatibility across
upgrades.

This fork publishes GitHub release archives for Linux, macOS, and Windows. It
does not publish a Homebrew formula, `.deb` or `.rpm` packages, or a container
image.

On macOS or Linux, run the shell installer:

```sh
curl -fsSL https://raw.githubusercontent.com/ifigueroap/poomsae/main/scripts/install.sh | bash
```

On Windows PowerShell, run:

```powershell
powershell -ExecutionPolicy ByPass -c "irm https://raw.githubusercontent.com/ifigueroap/poomsae/main/scripts/install.ps1 | iex"
```

Both installers detect your OS and CPU architecture, download the latest
archive from [GitHub releases](https://github.com/ifigueroap/poomsae/releases), and
verify it against `SHA256SUMS` before installing. For v0.14.x and newer
versions, they also ask the extracted binary to validate its embedded Web UI
before installing the binary, while still supporting canonical older releases
that predate that check. If you prefer to inspect the scripts first,
read them at <https://raw.githubusercontent.com/ifigueroap/poomsae/main/scripts/install.sh> and
<https://raw.githubusercontent.com/ifigueroap/poomsae/main/scripts/install.ps1>.

The shell installer shows a download progress line when you run it in a
terminal. It aborts transfers that stall for 30 seconds and tries each download
up to three times. If every attempt fails, it suggests running the installer
again or downloading the release manually. The shell installer places `poomsae`
in `/usr/local/bin` or `~/.local/bin`, while the PowerShell installer places
`poomsae.exe` in `%USERPROFILE%\.poomsae\bin` by default and adds that directory
to your user `Path`.

## Updating Poomsae

Update poomsae with the same installer or package manager that owns your binary.
If a service manager runs your daemon, follow that deployment's update and
restart procedure.

After you install a release build, check for newer releases:

```sh
poomsae update --check
```

If you installed from an ordinary release archive or one of the installers,
install an available update from GitHub release assets:

```sh
poomsae update
```

`poomsae update` verifies the downloaded archive against `SHA256SUMS` before
replacing the installed binary. After the install succeeds, it asks a running
local daemon to restart itself with the new binary. The daemon restarts with
its own startup options and environment, so `--listen`, `--insecure-readonly`,
authentication tokens, and the auto-start idle timeout carry over without the
updater knowing them. A stopped daemon stays stopped. If the daemon used an
ephemeral port, reopen `poomsae ui`, because the replacement binds a new one.

If the daemon does not come back, or does not support automatic restart,
`poomsae update` reports that the binary was installed and returns an error. Fix
any reported startup problem, then run `poomsae daemon stop` followed by
`poomsae daemon start` with the daemon's original startup options in its original
host shell or service environment. After a package-manager upgrade,
`poomsae daemon restart` uses the executable invoked by the CLI while keeping the
running daemon's environment. Both processes must see the same process IDs and
executable paths. Older signal-only daemons need one stop and start before
explicit restart can do this.

Package-managed installations keep `poomsae update --check`, while commands that
install updates direct you back to the owning package manager. Packagers can
read the complete [packaging contract](../development/packaging.md). Installing
with `go install` remains supported, and building from a clone is still useful
for development builds.

For normal federated issue synchronization, upgrade the hub first. Spokes can
upgrade later, and you do not need to coordinate every user's installation. See
[Federation upgrades](../operations/federation.md#compatibility-and-rolling-upgrades)
for the procedure and the separate limits for projects that use cron.

Keep a [backup](../operations/backup-restore.md) before you upgrade a database.
For SQLite, stop other processes using that same database and let the new
daemon migrate it on startup. Remote spokes use their own databases and do not
need to stop for the hub's migration. If you run PostgreSQL, follow the
[upgrade procedure](../operations/postgres.md#upgrade-ceremony).

New client features may require a newer daemon API. Check the
[API version history](../reference/http-api.md#version-history) for details.
For changes specific to the versions you are crossing, including renamed
commands and cron compatibility, read the [release notes](../changelog.md).

## Install with `go install`

```sh
go install github.com/ifigueroap/poomsae/cmd/poomsae@latest
```

Installing from module source includes the CLI, daemon, and TUI, but Go does
not run the browser asset toolchain. To use `poomsae ui`, install a release binary
or build from a clone with `make install` so the compiled browser bundle is
embedded.

For a tagged Go install, `poomsae update` replaces the binary with a release archive.

Go writes the binary to `$(go env GOBIN)` when that variable is set, or to
`$(go env GOPATH)/bin` otherwise. Common defaults are `~/go/bin` on Unix and
`%USERPROFILE%\go\bin` on Windows. Add that directory to your `PATH`.

Verify the installation:

```sh
poomsae version   # or poomsae --version
poomsae --help
```

## Install from a release manually

Download the archive for your platform from the
[GitHub releases](https://github.com/ifigueroap/poomsae/releases) page, extract it,
and place the `poomsae` binary on your `PATH`.

## Build from a clone

On macOS or Linux, clone the repository and install with Make:

```sh
git clone https://github.com/ifigueroap/poomsae.git
cd poomsae
make install
```

`make install` honors `GOBIN` and defaults to `~/.local/bin`:

```sh
make install GOBIN=/usr/local/bin
```

On Windows, use PowerShell or `cmd.exe`:

```powershell
git clone https://github.com/ifigueroap/poomsae.git
cd poomsae
go build -o poomsae.exe ./cmd/poomsae
```

Move `poomsae.exe` to a directory on your `PATH`.

## Documentation tooling

This site builds with Zensical. Install the docs toolchain into a local virtual
environment:

```sh
make docs-install
```

Build or preview the site locally:

```sh
make docs-build
make docs-serve
```

Run `make docs-check` to run the repository's docs structure check followed by
a strict Zensical build when Zensical is installed.
