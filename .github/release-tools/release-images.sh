#!/usr/bin/env bash
set -euo pipefail
: "${RELEASE_VERSION:?}" "${RELEASE_SHA:?}" "${GITHUB_REPOSITORY:?}" "${RUNNER_TEMP:?}"
registry=junxuanb/sub2api
for arch in amd64 arm64; do
  args=(--platform "linux/$arch" --file ".release-context/$arch/Dockerfile"
    --label "org.opencontainers.image.version=$RELEASE_VERSION"
    --label "org.opencontainers.image.revision=$RELEASE_SHA"
    --label "org.opencontainers.image.source=https://github.com/$GITHUB_REPOSITORY"
    --tag "$registry:$RELEASE_VERSION-$arch")
  if [[ ${DRY_RUN:-false} == true ]]; then
    args+=(--output "type=oci,dest=$RUNNER_TEMP/sub2api-$arch.oci.tar")
  else
    args+=(--push)
  fi
  docker buildx build "${args[@]}" ".release-context/$arch"
done
if [[ ${DRY_RUN:-false} != true ]]; then
  major=${RELEASE_VERSION%%.*}
  minor=${RELEASE_VERSION#*.}; minor=${minor%%.*}
  docker buildx imagetools create \
    --tag "$registry:$RELEASE_VERSION" --tag "$registry:latest" \
    --tag "$registry:$major.$minor" --tag "$registry:$major" \
    "$registry:$RELEASE_VERSION-amd64" "$registry:$RELEASE_VERSION-arm64"
fi
