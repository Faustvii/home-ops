#!/usr/bin/env bash
# Emits dummy cluster-secrets Secrets (one per namespace) for `flate --supply -`,
# because the real ones are SOPS-encrypted and unreadable offline.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

keys=(
    HASS_IP
    NFS_PATH_CALIBREAUTOMATED_INGEST
    NFS_PATH_CALIBREAUTOMATED_LIB
    NFS_PATH_DOWNLOADS
    NFS_PATH_K8S
    NFS_PATH_MEDIA
    NFS_PATH_MEDIA_ANIME
    NFS_PATH_MEDIA_BOOKS
    NFS_PATH_MEDIA_MOVIES
    NFS_PATH_MEDIA_SERIES
    NFS_PATH_NEXTCLOUD
    NFS_PATH_PLEXMEDIA
    NFS_SERVER_IP
    SECRET_DOMAIN
    SECRET_DOMAIN_1
    TZ
)

namespaces=(flux-system)
for dir in "${root}"/kubernetes/apps/*/; do
    namespaces+=("$(basename "${dir}")")
done

printf '%s\n' "${namespaces[@]}" | sort -u | while read -r ns; do
    printf -- '---\napiVersion: v1\nkind: Secret\nmetadata:\n  name: cluster-secrets\n  namespace: %s\nstringData:\n' "${ns}"
    for key in "${keys[@]}"; do
        printf '  %s: dummy\n' "${key}"
    done
done
