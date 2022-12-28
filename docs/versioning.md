# Versioning

Oasis Web3 Gateway uses [Semantic Versioning 2.0.0] with the following
format:

```text
MAJOR.MINOR.PATCH[-MODIFIER]
```

where:

- `MAJOR` represents the major version starting with zero (e.g. `0`, `1`, `2`,
  `3`, ...),
- `MINOR` represents the minor version starting with zero (e.g. `0`, `1`, `2`,
  `3`, ...),
- `PATCH` represents the final number in the version (sometimes referred
  to as the "micro" segment) (e.g. `0`, `1`, `2`, `3`, ...).
- `MODIFIER` represents (optional) build metadata, e.g. `git8c01382`.

When a backwards incompatible release is made, the `MAJOR` version should be
bumped.

When a regularly scheduled release is made, the `MINOR` version should be
bumped.

If there are fixes and (backwards compatible) changes that we want to back-port
from an upcoming release, then the `PATCH` version should be bumped.

The `MODIFIER` should be used to denote a build from an untagged (and
potentially unclean) git source. It should be of the form:

```text
gitCOMMIT_SHA[+dirty]
```

where:

- `COMMIT_SHA` represents the current commit’s abbreviated SHA.

The `+dirty` part is optional and is only present if there are uncommitted
changes in the working directory.

[Semantic Versioning 2.0.0]: https://semver.org/spec/v2.0.0.html
