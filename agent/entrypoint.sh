#!/bin/sh
# Auto-detect the GIDs of mounted resources (Docker socket, systemd journal)
# so the agent can access them without requiring the operator to pre-configure
# host group IDs. Drops from root to UID/GID 65532 (nonroot) before exec.
set -e

GROUPS_ARG=""

if [ -e /var/run/docker.sock ]; then
    DOCKER_GID=$(stat -c '%g' /var/run/docker.sock)
    GROUPS_ARG="$DOCKER_GID"
fi

if [ -d /run/log/journal ]; then
    JOURNAL_GID=$(stat -c '%g' /run/log/journal)
    if [ -n "$GROUPS_ARG" ]; then
        GROUPS_ARG="$GROUPS_ARG,$JOURNAL_GID"
    else
        GROUPS_ARG="$JOURNAL_GID"
    fi
fi

if [ -n "$GROUPS_ARG" ]; then
    exec setpriv --reuid=65532 --regid=65532 --clear-groups --groups="$GROUPS_ARG" /blackbox-agent "$@"
else
    exec setpriv --reuid=65532 --regid=65532 --clear-groups /blackbox-agent "$@"
fi
