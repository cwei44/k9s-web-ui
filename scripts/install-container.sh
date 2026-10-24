#!/bin/sh
# Build the UI image and extract its static host helper. Run on the Docker host.
set -eu
if [ "$(id -u)" != 0 ]; then
    echo 'Run as root in the host environment where kubectl works.' >&2
    exit 1
fi
command -v docker >/dev/null 2>&1 || { echo 'Install Docker Engine and CLI first.' >&2; exit 1; }
command -v kubectl >/dev/null 2>&1 || { echo 'Host kubectl must already be installed.' >&2; exit 1; }
case "${DOCKER_HOST:-}" in ''|unix://*) ;; *) echo 'A local Linux Docker daemon is required.' >&2; exit 1 ;; esac
case "${DOCKER_CONTEXT:-default}" in ''|default) ;; *) echo 'Use the local default Docker context.' >&2; exit 1 ;; esac
repo=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
image=${K9S_WEB_UI_IMAGE:-k9s-web-ui:local}
if [ "${K9S_WEB_UI_SKIP_BUILD:-0}" != 1 ]; then
    docker build -t "$image" "$repo"
fi
temp_dir=$(mktemp -d)
extract_container=
cleanup() {
    if [ -n "$extract_container" ]; then docker rm "$extract_container" >/dev/null 2>&1 || :; fi
    rm -rf "$temp_dir"
}
trap cleanup EXIT
trap 'exit 1' HUP INT TERM
extract_container=$(docker create "$image" version)
docker cp "$extract_container:/k9s-web-ui" "$temp_dir/k9s-web-ui"
chmod 755 "$temp_dir/k9s-web-ui"
"$temp_dir/k9s-web-ui" container-install --image "$image" "$@"
