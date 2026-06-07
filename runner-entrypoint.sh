#!/bin/sh
# Runner entrypoint: selects the platform-specific runner binary
case "${RUNNER_PLATFORM}" in
  codeup)
    exec /runners/codeup-runner
    ;;
  *)
    exec /runners/gitlab-runner
    ;;
esac
