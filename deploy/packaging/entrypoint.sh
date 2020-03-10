#!/usr/bin/env sh


set -e

# check to see if this file is being run or sourced from another script
_is_sourced() {
	# https://unix.stackexchange.com/a/215279
	[ "${#FUNCNAME[@]}" -ge 2 ] \
		&& [ "${FUNCNAME[0]}" = '_is_sourced' ] \
		&& [ "${FUNCNAME[1]}" = 'source' ]
}

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
		  /spruce merge --prune service.fixed /talaria.yaml /tmp/talaria_spruce.yaml > /etc/talaria/talaria.yaml
		fi
	fi

	exec "$@"
}

# If we are sourced from elsewhere, don't perform any further actions
if ! _is_sourced; then
	_main "$@"
fi