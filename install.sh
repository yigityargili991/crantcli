#!/bin/sh
set -eu

repo_url="https://github.com/yigityargili991/crantcli"
asset_prefix="crant_type_look"
binary_name="crantcli"
version="${CRANTCLI_VERSION:-latest}"
tmp_dir=""

log() {
	printf '%s\n' "$*"
}

warn() {
	printf 'warning: %s\n' "$*" >&2
}

die() {
	printf 'error: %s\n' "$*" >&2
	exit 1
}

cleanup() {
	if [ -n "$tmp_dir" ] && [ -d "$tmp_dir" ]; then
		rm -rf "$tmp_dir"
	fi
}

command_exists() {
	command -v "$1" >/dev/null 2>&1
}

download() {
	url=$1
	output=$2

	if command_exists curl; then
		curl -fsSL "$url" -o "$output"
	elif command_exists wget; then
		wget -q "$url" -O "$output"
	else
		die "curl or wget is required to download $binary_name"
	fi
}

download_optional() {
	url=$1
	output=$2

	if command_exists curl; then
		curl -fsSL "$url" -o "$output" >/dev/null 2>&1
	elif command_exists wget; then
		wget -q "$url" -O "$output" >/dev/null 2>&1
	else
		return 1
	fi
}

sha256_file() {
	file=$1

	if command_exists sha256sum; then
		sha256sum "$file" | awk '{ print $1 }'
	elif command_exists shasum; then
		shasum -a 256 "$file" | awk '{ print $1 }'
	else
		return 1
	fi
}

uname_s=$(uname -s 2>/dev/null || printf unknown)
case "$uname_s" in
	Darwin)
		os=darwin
		;;
	Linux)
		os=linux
		;;
	*)
		die "unsupported operating system: $uname_s"
		;;
esac

uname_m=$(uname -m 2>/dev/null || printf unknown)
case "$uname_m" in
	x86_64 | amd64)
		arch=amd64
		;;
	arm64 | aarch64)
		arch=arm64
		;;
	*)
		die "unsupported architecture: $uname_m"
		;;
esac

if [ "${CRANTCLI_INSTALL_DIR+x}" = x ]; then
	install_dir=$CRANTCLI_INSTALL_DIR
else
	if [ -z "${HOME:-}" ]; then
		die "HOME is not set; set CRANTCLI_INSTALL_DIR to choose an install directory"
	fi
	install_dir="$HOME/.local/bin"
fi

case "$version" in
	latest)
		release_base="$repo_url/releases/latest/download"
		;;
	*)
		release_base="$repo_url/releases/download/$version"
		;;
esac

asset="$asset_prefix-$os-$arch"
asset_url="$release_base/$asset"
checksums_url="$release_base/checksums.txt"

tmp_dir=$(mktemp -d 2>/dev/null || mktemp -d -t crantcli)
trap cleanup 0
trap 'cleanup; exit 1' HUP INT TERM

log "Installing $binary_name $version for $os/$arch"
download "$asset_url" "$tmp_dir/$asset"

if download_optional "$checksums_url" "$tmp_dir/checksums.txt"; then
	expected_hash=$(awk -v asset="$asset" '$2 == asset { print $1; found = 1; exit } END { if (!found) exit 1 }' "$tmp_dir/checksums.txt") || {
		die "checksums.txt does not contain an entry for $asset"
	}

	if actual_hash=$(sha256_file "$tmp_dir/$asset"); then
		if [ "$actual_hash" != "$expected_hash" ]; then
			die "checksum mismatch for $asset"
		fi
		log "Verified checksum for $asset"
	else
		warn "checksums.txt found, but sha256sum/shasum is unavailable; skipping checksum verification"
	fi
else
	warn "checksums.txt not found for $version; skipping checksum verification"
fi

mkdir -p "$install_dir" || die "could not create install directory: $install_dir"
if [ ! -w "$install_dir" ]; then
	die "install directory is not writable: $install_dir"
fi

install_path="$install_dir/$binary_name"
cp "$tmp_dir/$asset" "$install_path" || die "could not install $binary_name to $install_path"
chmod 0755 "$install_path"

log "Installed $binary_name to $install_path"

case ":${PATH:-}:" in
	*:"$install_dir":*)
		;;
	*)
		warn "$install_dir is not on PATH; add it to your shell profile to run '$binary_name' directly"
		;;
esac

if [ "$os" = linux ]; then
	if ! command_exists wl-copy && ! command_exists xclip && ! command_exists xsel; then
		warn "clipboard workflows need wl-clipboard, xclip, or xsel on Linux"
	fi
fi

log "Next: $binary_name setup"
