#!/bin/sh
# Installs the latest yure release.
#
#   curl -fsSL https://raw.githubusercontent.com/torihazi/yure/main/install.sh | sh
#
# Set YURE_INSTALL_DIR to choose where the binary goes (default: ~/.local/bin),
# or YURE_VERSION to install a specific version (e.g. v0.1.0).
set -eu

repo="torihazi/yure"
dir="${YURE_INSTALL_DIR:-$HOME/.local/bin}"
version="${YURE_VERSION:-latest}"

fail() {
	echo "yure: $*" >&2
	exit 1
}

case "$(uname -s)" in
Darwin) os=darwin ;;
Linux) os=linux ;;
*) fail "unsupported OS: $(uname -s) (on Windows, download the zip from https://github.com/$repo/releases)" ;;
esac
case "$(uname -m)" in
x86_64 | amd64) arch=amd64 ;;
arm64 | aarch64) arch=arm64 ;;
*) fail "unsupported CPU: $(uname -m)" ;;
esac

if [ "$version" = latest ]; then
	# The latest release URL redirects to .../releases/tag/vX.Y.Z.
	version=$(curl -fsSLI -o /dev/null -w '%{url_effective}' "https://github.com/$repo/releases/latest")
	version=${version##*/}
fi
case "$version" in
v[0-9]*) ;;
*) fail "could not determine the version to install (got \"$version\")" ;;
esac

archive="yure_${version#v}_${os}_${arch}.tar.gz"
base="https://github.com/$repo/releases/download/$version"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

echo "Downloading yure $version ($os/$arch)..."
curl -fsSL -o "$tmp/$archive" "$base/$archive" ||
	fail "no $os/$arch build of yure $version (see https://github.com/$repo/releases)"
curl -fsSL -o "$tmp/checksums.txt" "$base/checksums.txt" || fail "could not download checksums.txt"

want=$(awk -v f="$archive" '$2 == f { print $1 }' "$tmp/checksums.txt")
if command -v sha256sum >/dev/null 2>&1; then
	got=$(sha256sum "$tmp/$archive" | awk '{ print $1 }')
else
	got=$(shasum -a 256 "$tmp/$archive" | awk '{ print $1 }')
fi
if [ -z "$want" ] || [ "$want" != "$got" ]; then
	fail "checksum mismatch for $archive"
fi

tar -xzf "$tmp/$archive" -C "$tmp" yure
mkdir -p "$dir"
install -m 755 "$tmp/yure" "$dir/yure"
echo "Installed yure $version to $dir/yure"

case ":$PATH:" in
*":$dir:"*) echo "Run 'yure --demo' to try it." ;;
*) echo "Add $dir to your PATH to run it, e.g.: export PATH=\"$dir:\$PATH\"" ;;
esac
