# Repository Governance

## Protected branch policy

The canonical implementation branch is `main`.

The intended GitHub branch protection / ruleset is:

```text
target branch                         main
require pull request before merge    yes
required approving reviews           0 (solo-maintainer baseline)
require conversation resolution      yes
require status checks                 yes
require branch up to date             yes
block force pushes                    yes
block branch deletion                 yes
allow bypass                          repository administrator only
```

Required CI checks:

```text
test (ubuntu-latest)
test (macos-latest)
test (windows-latest)
race
```

The manually dispatched `long-duration-soak` workflow is deliberately **not** a required status check. Its 1h+ profiles are empirical release/reference gates and should not block every normal pull request.

## Why zero required approvals initially?

The repository currently has a solo-maintainer workflow. Requiring one approving review would prevent the author from merging their own pull request without adding another reviewer merely to satisfy mechanics.

The important initial invariant is therefore:

```text
all changes flow through a PR
+ required CI passes
+ branch is current
+ discussions are resolved
+ main cannot be force-pushed or deleted
```

When active collaborators are added, increase required approving reviews to at least one and optionally enable CODEOWNERS-based review requirements.

## Generation gates

Closing an implementation generation requires:

1. implementation and killer/invariant tests;
2. cross-platform CI and race validation where applicable;
3. an exit review under `docs/G*_EXIT_REVIEW.md`;
4. `docs/CURRENT_GENERATION.md`, `README.md` and `docs/ROADMAP.md` updated in the same PR;
5. any deferred empirical validation explicitly recorded rather than described as completed.

## Long-duration validation

Long-session empirical gates are defined in `LONG_DURATION_BENCHMARKS.md`.

The normal CI compiles tagged soak scenarios to prevent bitrot. Representative 1h/8h/24h executions remain separate because they are unsuitable as mandatory checks on every pull request.
