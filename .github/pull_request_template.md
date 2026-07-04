## Description

<!-- Provide a clear and concise description of the changes. -->

Fixes # (issue)

## Related Issues

<!-- Link any related issues or discussions. -->

## Type of Change

- [ ] `feat:` New feature
- [ ] `fix:` Bug fix
- [ ] `test:` Test addition or update
- [ ] `docs:` Documentation change
- [ ] `chore:` Build, CI, tooling, or dependency change
- [ ] `refactor:` Code restructuring without behavior change

## Breaking Changes

- [ ] This PR introduces a breaking change to the public API (`pkg/gmock/`)

> **Backwards Compatibility**: gochaos is a library-first project. Changes to
> `pkg/gmock/` must maintain backwards compatibility. If a breaking change is
> necessary, it must be called out explicitly and discussed in the PR before merge.

## Testing

<!-- Describe the tests you ran and how to reproduce them. -->

- [ ] Tests pass (`go test -race ./...`)
- [ ] New tests added for the change (or explain why not needed)

## Screenshots / Demo

<!-- Optional: for UI or CLI output changes, show before/after. Skip for non-visual changes. -->

## Checklist

- [ ] `go vet ./...` passes
- [ ] Documentation updated (README, feature guides, admin-api.md if applicable)
- [ ] CHANGELOG.md updated (if this is a user-visible change)
- [ ] Commit messages follow [conventional commit format](CONTRIBUTING.md#commit-messages)
