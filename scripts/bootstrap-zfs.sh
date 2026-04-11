#!/usr/bin/env bash
set -Eeuo pipefail

source "$(dirname "${0}")/lib/common.sh"

export LOG_LEVEL="${LOG_LEVEL:-debug}"

POOL_NAME="${POOL_NAME:-tank}"
PARTLABEL="${PARTLABEL:-r-openebs-zpool}"
NODE="${NODE:-talos-01}"
POOL_DISK="/dev/disk/by-partlabel/${PARTLABEL}"
KUBE_DEBUG_IMAGE="${KUBE_DEBUG_IMAGE:-busybox:1.36}"

function ensure_zpool() {
    log debug "Ensuring zpool ${POOL_NAME} exists using ${POOL_DISK} on ${NODE}"

    kubectl debug \
        node/"${NODE}" \
        -n kube-system \
        --image="${KUBE_DEBUG_IMAGE}" \
        --profile=sysadmin \
        -i \
        -- \
        sh -ec "
        if chroot /host zpool list -H -o name '${POOL_NAME}' >/dev/null 2>&1; then
            echo 'ZFS pool ${POOL_NAME} already present on ${NODE}; skipping'
            exit 0
        fi

        chroot /host zpool create \
            -m legacy \
            -o ashift=12 \
            -O compression=on \
            -O atime=off \
            '${POOL_NAME}' \
            '${POOL_DISK}'
    "
}

function main() {
    check_env KUBECONFIG
    check_cli kubectl
    ensure_zpool
}

main "$@"
