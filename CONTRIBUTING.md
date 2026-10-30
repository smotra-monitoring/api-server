# Contributing

Thank you for your interest in contributing. To keep quality high and maintainer time focused on real improvements, this project has strict intake rules. Pull requests that do not follow them are closed without review.

## Ground rules

- **Unsolicited PRs are not accepted.** Open an issue first and wait for a maintainer to approve the approach.
- **You are accountable for every line you submit.** You must be able to explain, justify and modify your change in review without an AI assistant. Using AI tools is allowed, but it must be disclosed in the PR, and unreviewed or bulk-generated output is not accepted.
- **One PR, one purpose.** Features, bug fixes and security fixes use separate PRs and separate templates.
- **CLA.** If your contribution is accepted for review, you must sign the Contributor License Agreement before it can be merged.

## Contribution types

| Type | How to contribute | PR template |
| --- | --- | --- |
| Feature | Via a skill file (`SKILL.md`), see below | [feature.md](.github/PULL_REQUEST_TEMPLATE/feature.md) |
| Bug fix | Report as a GitHub issue first, then fix | [bugfix.md](.github/PULL_REQUEST_TEMPLATE/bugfix.md) |
| Security | Full vulnerability report with reproduction | [security.md](.github/PULL_REQUEST_TEMPLATE/security.md) |

To open a PR with a specific template, append `?template=<file>` to the GitHub compare URL, for example `?template=bugfix.md`.

### Features: accepted only via `SKILL.md`

Feature PRs are accepted **only** as a skill file. Pull requests that add feature code without a skill file are closed.

1. Open a feature-request issue and get maintainer approval.
2. Add `.github/skills/<feature-name>/SKILL.md` describing the feature. See the existing skills in [.github/skills](.github/skills) for the format (YAML front matter with `name` and `description`, followed by the instructions).
3. The skill must state:
   - the purpose and scope of the feature, and what is explicitly out of scope;
   - affected components (API spec, database schema, handlers, config);
   - the step-by-step implementation procedure, following [.github/copilot-instructions.md](.github/copilot-instructions.md) and the files in [.github/instructions](.github/instructions);
   - required tests (unit and integration), metrics, and documentation updates;
   - acceptance criteria that a reviewer can verify.
4. Reference the approved issue and the skill file in the PR (the feature template enforces this).

### Bug fixes: submitted as GitHub issues

Bugs must be filed as a GitHub issue **before** any PR.

1. Search existing issues, then open a new one with: version or commit, environment, steps to reproduce, expected behaviour, actual behaviour, and relevant logs.
2. Wait for a maintainer to confirm the bug.
3. Submit a PR that links the issue (`Fixes #<number>`). The PR must contain a regression test that fails without the fix.

PRs for bugs that have no confirmed issue are closed.

### Security fixes

Do not publish exploit details before the maintainers have had a chance to fix the issue. Prefer GitHub private vulnerability reporting (Security tab, "Report a vulnerability") and include the same four items below. A security PR must contain **all** of the following:

1. **Detailed explanation of the vulnerability**: affected component and versions, root cause, impact, attacker prerequisites, and a severity assessment.
2. **Steps to reproduce**: exact, minimal, numbered steps starting from a clean checkout, including requests or payloads used.
3. **Unit test that reproduces the vulnerability**: a Go test that fails on the current code and passes with the fix.
4. **Proposed fix or mitigation**: the change itself or, if a full fix is not possible, a described mitigation and its limitations.

Security PRs missing any item are closed.

## Development workflow

1. Fork the repository and create a branch from `main`.
2. Make a focused change following the code standards below.
3. Run the checks locally:

   ```bash
   just fmt
   just lint
   just test
   ```

4. Use [Conventional Commits](https://www.conventionalcommits.org/) for commit messages (`feat:`, `fix:`, `docs:`, ...).
5. Open the PR with the matching template and complete every section.

## Code standards

- Follow standard Go conventions and idioms.
- Tests are mandatory: unit tests (`<name>_test.go`) and integration tests (`<name>_integration_test.go`) alongside the code. Bug fixes need a regression test.
- New features must add Prometheus metrics, as described in [.github/instructions/metrics.instructions.md](.github/instructions/metrics.instructions.md).
- Generated code (`api.gen.go`, sqlc output) must be regenerated with `just generate-oapi` and `just generate-sqlc`, never edited by hand.
- Update the documentation in [docs](docs) when behaviour changes.

See the [User Guide](docs/features/GUIDE.md) and [Testing guide](docs/TESTING.md) for details.

## What gets a PR closed

- No linked, maintainer-approved issue.
- Feature PR without a `SKILL.md`.
- Security PR missing the explanation, reproduction steps, reproducing unit test or proposed fix.
- Bug fix PR without a linked issue or a regression test.
- Changes that do not build, fail tests, or are unrelated to the linked issue.
- Contributor cannot explain the change when asked, or the AI disclosure is missing or false.
- Cosmetic, trivial or drive-by changes made only to collect a contribution.

## License

By contributing you agree that your contributions are licensed under the terms in [LICENSE](LICENSE).
