#!/bin/sh
# Auto-detect the GIDs of mounted resources (Docker socket, systemd journal)
# so the agent can access them without requiring the operator to pre-configure
# host group IDs. Drops from root to UID/GID 65532 (nonroot) before exec.
set -e

# Helper function to add unique GID to GROUPS_ARG
add_unique_gid() {
    if [ -z "$GROUPS_ARG" ]; then
        GROUPS_ARG="$1"
    elif ! echo "$GROUPS_ARG" | grep -qE "(^|,)$1(,|$)"; then
        GROUPS_ARG="$GROUPS_ARG,$1"
    fi
}

if [ -e /var/run/docker.sock ]; then
    DOCKER_GID=$(stat -c '%g' /var/run/docker.sock)
    add_unique_gid "$DOCKER_GID"
fi

# Check both common journal paths and add the first one found
if [ -d /run/log/journal ]; then
    JOURNAL_GID=$(stat -c '%g' /run/log/journal)
    add_unique_gid "$JOURNAL_GID"
elif [ -d /var/log/journal ]; then
    JOURNAL_GID=$(stat -c '%g' /var/log/journal)
    add_unique_gid "$JOURNAL_GID"
fi

if [ -n "$GROUPS_ARG" ]; then
    exec setpriv --reuid=65532 --regid=65532 --clear-groups --groups="$GROUPS_ARG" /blackbox-agent "$@"
else
    exec setpriv --reuid=65532 --regid=65532 --clear-groups /blackbox-agent "$@"
fi
