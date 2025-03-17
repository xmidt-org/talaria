#!/usr/bin/env sh
# SPDX-FileCopyrightText: 2022 Comcast Cable Communications Management, LLC
# SPDX-License-Identifier: Apache-2.0
set -e

# check arguments for an option that would cause /talaria to stop
# return true if there is one
_want_help() {
    local arg
    for arg; do
        case "$arg" in
            -'?'|--help|-v)
                return 0
                ;;
        esac
    done
    return 1
}

_main() {
    # if command starts with an option, prepend talaria
    if [ "${1:0:1}" = '-' ]; then
        set -- /talaria "$@"
    fi

    # skip setup if they aren't running /talaria or want an option that stops /talaria
    if [ "$1" = '/talaria' ] && ! _want_help "$@"; then
        echo "Entrypoint script for talaria Server ${VERSION} started."

        if [ ! -s /etc/talaria/talaria.yaml ]; then
            echo "Building out template for file"
            /bin/spruce merge /tmp/talaria_spruce.yaml > /etc/talaria/talaria.yaml
        fi
    fi

    exec "$@"
}

_main "$@"
