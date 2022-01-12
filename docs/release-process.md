# Release Process

The following steps are needed to create a release.

Create a tag on the commit that should be released:

```bash
export VERSION=v0.0.1
git tag --sign --message="Version ${VERSION}" "${VERSION}"
```

Push the tag upstream:

```bash
git push origin "${VERSION}"
```

This will trigger the release action that will create a and publish a release,
together with the changelog with changes since the previous release. Commit
messages are used to generate the changelog.
