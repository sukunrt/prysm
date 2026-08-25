#!/bin/bash

# Note: The STABLE_ prefix will force a relink when the value changes when using rules_go x_defs.
# Fall back to placeholders when git metadata is unavailable (e.g. a jj checkout with no .git).

commit="$(git rev-parse HEAD 2>/dev/null || echo unknown)"
branch="$(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo unknown)"
short="$(git rev-parse --short=6 HEAD 2>/dev/null || echo unknown)"
tag="$(git describe --tags --abbrev=0 2>/dev/null || echo v0.0.0-dev)"

echo STABLE_GIT_COMMIT "${commit}"
echo DATE "$(date --rfc-3339=seconds --utc)"
echo DATE_UNIX "$(date --utc +%s)"
echo DOCKER_TAG "${branch}-${short}"
echo STABLE_GIT_TAG "${tag}"
