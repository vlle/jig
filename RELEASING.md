# Releasing jig

A release is a `vX.Y.Z` tag on `main`. The `release` workflow tests the tag, builds Linux
and macOS binaries for amd64 and arm64 with GoReleaser and publishes them with
`checksums.txt` and the matching section of CHANGELOG.md as release notes.

## Ready to tag

Every box is a yes/no question with a way to check it.

### Quality

- [ ] CI on the release commit is green: tests on Linux (minimum Go from go.mod and stable)
      and macOS, `golangci-lint`, `go mod tidy -diff`, GoReleaser snapshot.
      Check: `gh run list --branch main --limit 1`.
- [ ] Locally: `go vet ./... && go test -race ./... && golangci-lint run` passes.
- [ ] Every user-visible change has an end-to-end test in `e2e/`.

### Works for someone who is not us

- [ ] A fresh workspace is clean: `jig init` in an empty git repository, then `jig doctor`
      exits 0.
- [ ] The adoption path works: an existing `.sh`, `.py` and Go tool registered with
      `jig add` run through `jig run` without editing the manifest.
- [ ] `jig new` scaffolds of every kind start with `jig run` in a workspace without `go.mod`.
- [ ] `jig doctor` exits 1 on a problem and 0 on a clean registry; `jig index` output does
      not change when run twice.
- [ ] `jig version` prints the version outside a workspace.
- [ ] The install instructions in README.md work as written, both `go install` and the
      prebuilt binary.

### Compatibility

- [ ] The `--json` output of `ls`, `show` and `doctor` only gained fields, or the major
      version goes up.
- [ ] Manifests that loaded in the previous release still load.
- [ ] Anything a user has to do after upgrading is written in the CHANGELOG.

### Documentation

- [ ] CHANGELOG.md: the `Unreleased` section is renamed to the version and dated, and the
      compare link at the bottom is added.
- [ ] `.claude-plugin/plugin.json` has the same version; the plugin launcher downloads the
      release with that number. `TestPluginManifest` fails until they match.
- [ ] `claude plugin validate .` passes.
- [ ] README.md mentions every new command and flag; `jig help` matches.
- [ ] `jig agent rules` and `jig agent skill` describe the current workflow.

## Cutting the release

```bash
git switch main && git pull
git tag -a v1.2.0 -m v1.2.0
git push origin v1.2.0
gh run watch "$(gh run list --workflow release --limit 1 --json databaseId -q '.[0].databaseId')"
```

Then check the release page: four archives, `checksums.txt`, notes from the CHANGELOG.
Finally, from outside the repository:

```bash
cd "$(mktemp -d)" && go install github.com/vlle/jig@v1.2.0 && jig version
```

And the plugin: `claude plugin marketplace update jig && claude plugin update jig@jig`, then
in a new session inside a workspace `jig version` from Claude's shell prints the new version
(with no other `jig` on PATH).

If the workflow fails, fix it on `main`, delete the tag (`git push --delete origin v1.2.0`,
`git tag -d v1.2.0`) and tag again. A tag that `go install` has already fetched is cached
by the Go module proxy forever; never move it, release the next patch version instead.
