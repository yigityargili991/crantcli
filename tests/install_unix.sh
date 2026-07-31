#!/bin/sh
set -eu

repository_root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
test_root=$(mktemp -d 2>/dev/null || mktemp -d -t crantcli-installer-test)
fixtures="$test_root/fixtures"
fake_bin="$test_root/bin"
download_log="$test_root/downloads.log"
cosign_log="$test_root/cosign.log"
verifier_log="$test_root/verifier.log"

cleanup() {
	rm -rf "$test_root"
}
trap cleanup 0
trap 'cleanup; exit 1' HUP INT TERM

mkdir -p "$fixtures" "$fake_bin"

fail() {
	printf 'error: %s\n' "$*" >&2
	exit 1
}

sha256_file() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | awk '{ print $1 }'
	elif command -v shasum >/dev/null 2>&1; then
		shasum -a 256 "$1" | awk '{ print $1 }'
	else
		fail "sha256sum or shasum is required"
	fi
}

for os in linux darwin; do
	for arch in amd64 arm64; do
		asset="crant_type_look-$os-$arch"
		printf '%s fixture\n' "$asset" >"$fixtures/$asset"
	done
done

write_checksums() {
	: >"$fixtures/checksums.txt"
	for asset_path in "$fixtures"/crant_type_look-*; do
		asset=${asset_path##*/}
		printf '%s  %s\n' "$(sha256_file "$asset_path")" "$asset" >>"$fixtures/checksums.txt"
	done
}

write_checksums
for asset_path in "$fixtures"/crant_type_look-*; do
	asset=${asset_path##*/}
	printf '{"fixture":"%s"}\n' "$asset" >"$fixtures/$asset.sigstore.json"
	printf '{"fixture":"%s"}\n' "$asset" >"$fixtures/$asset.bundle.json"
done

cat >"$fake_bin/uname" <<'EOF'
#!/bin/sh
case "${1:-}" in
	-s) printf '%s\n' "$CRANTCLI_TEST_UNAME_S" ;;
	-m) printf '%s\n' "$CRANTCLI_TEST_UNAME_M" ;;
	*) exit 1 ;;
esac
EOF

cat >"$fake_bin/curl" <<'EOF'
#!/bin/sh
url=""
output=""
while [ "$#" -gt 0 ]; do
	case "$1" in
		-o)
			output=$2
			shift 2
			;;
		http://* | https://*)
			url=$1
			shift
			;;
		*)
			shift
			;;
	esac
done
test -n "$url" && test -n "$output"
printf '%s\n' "$url" >>"$CRANTCLI_TEST_DOWNLOAD_LOG"
fixture="$CRANTCLI_TEST_FIXTURES/${url##*/}"
test -f "$fixture" || exit 22
cp "$fixture" "$output"
EOF

cat >"$fake_bin/cosign" <<'EOF'
#!/bin/sh
printf '%s\n' "$*" >>"$CRANTCLI_TEST_COSIGN_LOG"
test "${CRANTCLI_TEST_COSIGN_FAIL:-0}" != "1"
EOF

cat >"$fake_bin/crantcli-verifier" <<'EOF'
#!/bin/sh
printf '%s\n' "$*" >>"$CRANTCLI_TEST_VERIFIER_LOG"
test "${CRANTCLI_TEST_VERIFIER_FAIL:-0}" != "1"
EOF

chmod 0755 "$fake_bin/uname" "$fake_bin/curl" "$fake_bin/cosign" "$fake_bin/crantcli-verifier"

run_install() {
	system=$1
	machine=$2
	expected_os=$3
	expected_arch=$4
	version=$5
	verification_mode=${6:-cosign}
	verifier=""
	if [ "$verification_mode" = builtin ]; then
		verifier="$fake_bin/crantcli-verifier"
	fi
	install_dir="$test_root/install-$expected_os-$expected_arch"
	mkdir -p "$install_dir"
	printf '%s\n' "old fixture" >"$install_dir/crantcli"
	: >"$download_log"
	: >"$cosign_log"
	: >"$verifier_log"

	CRANTCLI_TEST_UNAME_S=$system \
	CRANTCLI_TEST_UNAME_M=$machine \
	CRANTCLI_TEST_FIXTURES=$fixtures \
	CRANTCLI_TEST_DOWNLOAD_LOG=$download_log \
	CRANTCLI_TEST_COSIGN_LOG=$cosign_log \
	CRANTCLI_TEST_VERIFIER_LOG=$verifier_log \
	CRANTCLI_INSTALL_DIR=$install_dir \
	CRANTCLI_VERSION=$version \
	CRANTCLI_REQUIRE_SIGNATURE= \
	CRANTCLI_VERIFY_BINARY=$verifier \
	CRANTCLI_GITHUB_TOKEN= \
	PATH="$fake_bin:$PATH" \
	sh "$repository_root/install.sh"

	asset="crant_type_look-$expected_os-$expected_arch"
	test "$(sha256_file "$fixtures/$asset")" = "$(sha256_file "$install_dir/crantcli")" ||
		fail "$asset was not installed"
	test -x "$install_dir/crantcli" || fail "$asset is not executable"

	if [ "$version" = latest ]; then
		release_path="/releases/latest/download/$asset"
	else
		release_path="/releases/download/$version/$asset"
	fi
	grep -F "$release_path" "$download_log" >/dev/null ||
		fail "installer used the wrong release URL for $version"
	if [ "$verification_mode" = builtin ]; then
		grep -F -- "__verify-release" "$verifier_log" >/dev/null ||
			fail "installer did not invoke the built-in verifier"
		grep -F -- "$asset.bundle.json" "$download_log" >/dev/null ||
			fail "installer did not download the standardized bundle"
		test ! -s "$cosign_log" || fail "installer invoked cosign despite a built-in verifier"
	else
		grep -F -- "--certificate-oidc-issuer https://token.actions.githubusercontent.com" "$cosign_log" >/dev/null ||
			fail "installer did not constrain the signature issuer"
		grep -F -- "--certificate-identity-regexp ^https://github\\.com/yigityargili991/crantcli/\\.github/workflows/release\\.yml@refs/tags/v[^/]+$" "$cosign_log" >/dev/null ||
			fail "installer did not constrain the signing workflow identity"
	fi
}

run_install Linux x86_64 linux amd64 latest
run_install Linux aarch64 linux arm64 v1.2.3
run_install Darwin x86_64 darwin amd64 latest
run_install Darwin arm64 darwin arm64 v1.2.3
run_install Linux x86_64 linux amd64 latest builtin

cp "$fixtures/checksums.txt" "$test_root/checksums.good"
awk '{ print "0000000000000000000000000000000000000000000000000000000000000000  " $2 }' \
	"$test_root/checksums.good" >"$fixtures/checksums.txt"
checksum_install_dir="$test_root/checksum-failure"
if CRANTCLI_TEST_UNAME_S=Linux \
	CRANTCLI_TEST_UNAME_M=x86_64 \
	CRANTCLI_TEST_FIXTURES=$fixtures \
	CRANTCLI_TEST_DOWNLOAD_LOG=$download_log \
	CRANTCLI_TEST_COSIGN_LOG=$cosign_log \
	CRANTCLI_INSTALL_DIR=$checksum_install_dir \
	CRANTCLI_VERSION=latest \
	CRANTCLI_REQUIRE_SIGNATURE= \
	CRANTCLI_GITHUB_TOKEN= \
	PATH="$fake_bin:$PATH" \
	sh "$repository_root/install.sh" >"$test_root/checksum.out" 2>&1; then
	fail "installer accepted an invalid checksum"
fi
test ! -e "$checksum_install_dir/crantcli" ||
	fail "installer copied a binary after checksum verification failed"
grep -F "checksum mismatch" "$test_root/checksum.out" >/dev/null ||
	fail "checksum failure did not report the mismatch"

cp "$test_root/checksums.good" "$fixtures/checksums.txt"
signature_install_dir="$test_root/signature-failure"
if CRANTCLI_TEST_UNAME_S=Linux \
	CRANTCLI_TEST_UNAME_M=x86_64 \
	CRANTCLI_TEST_FIXTURES=$fixtures \
	CRANTCLI_TEST_DOWNLOAD_LOG=$download_log \
	CRANTCLI_TEST_COSIGN_LOG=$cosign_log \
	CRANTCLI_TEST_COSIGN_FAIL=1 \
	CRANTCLI_INSTALL_DIR=$signature_install_dir \
	CRANTCLI_VERSION=latest \
	CRANTCLI_REQUIRE_SIGNATURE= \
	CRANTCLI_GITHUB_TOKEN= \
	PATH="$fake_bin:$PATH" \
	sh "$repository_root/install.sh" >"$test_root/signature.out" 2>&1; then
	fail "installer accepted an invalid signature"
fi
test ! -e "$signature_install_dir/crantcli" ||
	fail "installer copied a binary after signature verification failed"
grep -F "cosign signature verification failed" "$test_root/signature.out" >/dev/null ||
	fail "signature failure did not report the verification error"

builtin_signature_install_dir="$test_root/builtin-signature-failure"
if CRANTCLI_TEST_UNAME_S=Linux \
	CRANTCLI_TEST_UNAME_M=x86_64 \
	CRANTCLI_TEST_FIXTURES=$fixtures \
	CRANTCLI_TEST_DOWNLOAD_LOG=$download_log \
	CRANTCLI_TEST_VERIFIER_LOG=$verifier_log \
	CRANTCLI_TEST_VERIFIER_FAIL=1 \
	CRANTCLI_INSTALL_DIR=$builtin_signature_install_dir \
	CRANTCLI_VERSION=latest \
	CRANTCLI_REQUIRE_SIGNATURE=1 \
	CRANTCLI_VERIFY_BINARY="$fake_bin/crantcli-verifier" \
	CRANTCLI_GITHUB_TOKEN= \
	PATH="$fake_bin:$PATH" \
	sh "$repository_root/install.sh" >"$test_root/builtin-signature.out" 2>&1; then
	fail "installer accepted a signature rejected by the built-in verifier"
fi
test ! -e "$builtin_signature_install_dir/crantcli" ||
	fail "installer copied a binary after built-in signature verification failed"
grep -F "Sigstore signature verification failed" "$test_root/builtin-signature.out" >/dev/null ||
	fail "built-in signature failure did not report the verification error"

required_standard_bundle="$fixtures/crant_type_look-linux-amd64.bundle.json"
saved_standard_bundle="$test_root/crant_type_look-linux-amd64.bundle.json"
mv "$required_standard_bundle" "$saved_standard_bundle"
missing_standard_bundle_dir="$test_root/missing-standard-bundle-failure"
if CRANTCLI_TEST_UNAME_S=Linux \
	CRANTCLI_TEST_UNAME_M=x86_64 \
	CRANTCLI_TEST_FIXTURES=$fixtures \
	CRANTCLI_TEST_DOWNLOAD_LOG=$download_log \
	CRANTCLI_INSTALL_DIR=$missing_standard_bundle_dir \
	CRANTCLI_VERSION=latest \
	CRANTCLI_REQUIRE_SIGNATURE=1 \
	CRANTCLI_VERIFY_BINARY="$fake_bin/crantcli-verifier" \
	CRANTCLI_GITHUB_TOKEN= \
	PATH="$fake_bin:$PATH" \
	sh "$repository_root/install.sh" >"$test_root/missing-standard-bundle.out" 2>&1; then
	mv "$saved_standard_bundle" "$required_standard_bundle"
	fail "update mode accepted a binary without a standardized bundle"
fi
mv "$saved_standard_bundle" "$required_standard_bundle"
test ! -e "$missing_standard_bundle_dir/crantcli" ||
	fail "installer copied a binary without its required standardized bundle"
grep -F "refusing an unauthenticated update" "$test_root/missing-standard-bundle.out" >/dev/null ||
	fail "missing standardized bundle did not report a fail-closed error"

required_bundle="$fixtures/crant_type_look-linux-amd64.sigstore.json"
saved_bundle="$test_root/crant_type_look-linux-amd64.sigstore.json"
mv "$required_bundle" "$saved_bundle"
missing_bundle_install_dir="$test_root/missing-bundle-failure"
if CRANTCLI_TEST_UNAME_S=Linux \
	CRANTCLI_TEST_UNAME_M=x86_64 \
	CRANTCLI_TEST_FIXTURES=$fixtures \
	CRANTCLI_TEST_DOWNLOAD_LOG=$download_log \
	CRANTCLI_TEST_COSIGN_LOG=$cosign_log \
	CRANTCLI_INSTALL_DIR=$missing_bundle_install_dir \
	CRANTCLI_VERSION=latest \
	CRANTCLI_REQUIRE_SIGNATURE=1 \
	CRANTCLI_GITHUB_TOKEN= \
	PATH="$fake_bin:$PATH" \
	sh "$repository_root/install.sh" >"$test_root/missing-bundle.out" 2>&1; then
	mv "$saved_bundle" "$required_bundle"
	fail "update mode accepted a binary without a signature bundle"
fi
mv "$saved_bundle" "$required_bundle"
test ! -e "$missing_bundle_install_dir/crantcli" ||
	fail "installer copied a binary without its required signature bundle"
grep -F "refusing an unauthenticated update" "$test_root/missing-bundle.out" >/dev/null ||
	fail "missing required signature bundle did not report a fail-closed error"

printf '%s\n' "Unix installer tests passed"
