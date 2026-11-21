#!/bin/sh
# Read Git metadata once per Codex launch, never once per model request.
set -eu

repository_dir=.
expect_dir=false
for arg in "$@"; do
    if [ "$expect_dir" = true ]; then
        repository_dir=$arg
        expect_dir=false
        continue
    fi
    case "$arg" in
        --) break ;;
        -C|--cd) expect_dir=true ;;
        --cd=*) repository_dir=${arg#--cd=} ;;
        -C?*) repository_dir=${arg#-C} ;;
    esac
done

# Clear inherited attribution when launched outside a repo or without origin.
unset SUB2API_REPOSITORY
repository_remote=$(git -C "$repository_dir" remote get-url origin 2>/dev/null || true)
if [ -n "$repository_remote" ]; then
    # Strip URL userinfo and query/fragment locally as well as on the server.
    SUB2API_REPOSITORY=$(printf '%s' "$repository_remote" | sed -E 's#(https?://)[^/@]*@#\1#; s#[?#].*$##')
    export SUB2API_REPOSITORY
fi
exec codex "$@"
