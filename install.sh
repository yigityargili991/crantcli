#!/bin/sh
set -eu

repo_url="https://github.com/yigityargili991/crantcli"
repo_slug="yigityargili991/crantcli"
asset_prefix="crant_type_look"
binary_name="crantcli"
version="${CRANTCLI_VERSION:-latest}"
github_token="${CRANTCLI_GITHUB_TOKEN:-}"
unset CRANTCLI_GITHUB_TOKEN
tmp_dir=""
stage_path=""

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
	if [ -n "$stage_path" ] && [ -e "$stage_path" ]; then
		rm -f "$stage_path"
	fi
}

command_exists() {
	command -v "$1" >/dev/null 2>&1
}

download() {
	url=$1
	output=$2

	if [ -n "$github_token" ]; then
		if ! command_exists gh; then
			die "gh is required when CRANTCLI_GITHUB_TOKEN is set"
		fi
		download_asset_name=${url##*/}
		if [ "$version" = latest ]; then
			GH_TOKEN=$github_token gh release download \
				--repo "$repo_slug" --pattern "$download_asset_name" --output "$output"
		else
			GH_TOKEN=$github_token gh release download "$version" \
				--repo "$repo_slug" --pattern "$download_asset_name" --output "$output"
		fi
	elif command_exists curl; then
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

	if [ -n "$github_token" ]; then
		if ! command_exists gh; then
			return 1
		fi
		download_asset_name=${url##*/}
		if [ "$version" = latest ]; then
			GH_TOKEN=$github_token gh release download \
				--repo "$repo_slug" --pattern "$download_asset_name" --output "$output" >/dev/null 2>&1
		else
			GH_TOKEN=$github_token gh release download "$version" \
				--repo "$repo_slug" --pattern "$download_asset_name" --output "$output" >/dev/null 2>&1
		fi
	elif command_exists curl; then
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

if [ "${CRANTCLI_SKIP_CHECKSUM:-}" = "1" ]; then
	warn "CRANTCLI_SKIP_CHECKSUM=1 set; skipping checksum verification (insecure)"
else
	# Fail closed: never install a binary whose integrity could not be verified.
	download "$checksums_url" "$tmp_dir/checksums.txt" ||
		die "could not download checksums.txt for $version; refusing to install an unverified binary (set CRANTCLI_SKIP_CHECKSUM=1 to override)"

	expected_hash=$(awk -v asset="$asset" '$2 == asset { print $1; found = 1; exit } END { if (!found) exit 1 }' "$tmp_dir/checksums.txt") || {
		die "checksums.txt does not contain an entry for $asset"
	}

	if ! actual_hash=$(sha256_file "$tmp_dir/$asset"); then
		die "sha256sum/shasum is unavailable; cannot verify $asset (set CRANTCLI_SKIP_CHECKSUM=1 to override)"
	fi
	if [ "$actual_hash" != "$expected_hash" ]; then
		die "checksum mismatch for $asset"
	fi
	log "Verified checksum for $asset"
fi

# Updates set CRANTCLI_REQUIRE_SIGNATURE=1 and fail closed unless the binary's
# keyless signature bundle can be authenticated. Direct installs retain the
# checksum-only fallback for older releases and environments without cosign.
if command_exists cosign; then
	bundle_url="$release_base/$asset.sigstore.json"
	if download_optional "$bundle_url" "$tmp_dir/$asset.sigstore.json"; then
		if cosign verify-blob --bundle "$tmp_dir/$asset.sigstore.json" \
			--certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
			--certificate-identity-regexp "^https://github\.com/yigityargili991/crantcli/\.github/workflows/release\.yml@refs/tags/v[^/]+$" \
			"$tmp_dir/$asset" >/dev/null 2>&1; then
			log "Verified cosign signature for $asset"
		else
			die "cosign signature verification failed for $asset"
		fi
	elif [ "${CRANTCLI_REQUIRE_SIGNATURE:-}" = "1" ]; then
		die "could not download the cosign signature bundle for $asset; refusing an unauthenticated update"
	else
		warn "no cosign signature bundle found for $version; relying on checksum verification"
	fi
elif [ "${CRANTCLI_REQUIRE_SIGNATURE:-}" = "1" ]; then
	die "cosign is required to authenticate update binaries"
else
	warn "cosign not installed; relying on checksum verification (install cosign for signature verification)"
fi

mkdir -p "$install_dir" || die "could not create install directory: $install_dir"
if [ ! -w "$install_dir" ]; then
	die "install directory is not writable: $install_dir"
fi

install_path="$install_dir/$binary_name"
stage_path="$install_dir/.$binary_name.new.$$"
cp "$tmp_dir/$asset" "$stage_path" || die "could not stage $binary_name in $install_dir"
chmod 0755 "$stage_path"
mv -f "$stage_path" "$install_path" || die "could not install $binary_name to $install_path"
stage_path=""

log "Installed $binary_name to $install_path"

case ":${PATH:-}:" in
	*:"$install_dir":*)
		;;
	*)
		warn "$install_dir is not on PATH; add it to your shell profile to run '$binary_name' directly"
		;;
esac

log "Next: $binary_name setup"
