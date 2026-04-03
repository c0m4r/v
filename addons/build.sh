#!/usr/bin/env bash
# build.sh — cross-compile v for amd64, arm64, riscv64
# Produces stripped, size-optimized static binaries in ./dist/

set -euo pipefail

cd "$(dirname "$0")/.."

BINARY="v"
MODULE="v"
DIST="dist"
VERSION="${VERSION:-$(cat VERSION)}"
DATE="${DATE:-$(date -u +%Y-%m-%d)}"

TARGETS=(
    "linux/amd64"
    "linux/arm64"
    "linux/riscv64"
)

# Flags injected at link time
LDFLAGS="-s -w \
  -X 'main.version=${VERSION}' \
  -X 'main.buildDate=${DATE}'"

# Extra gcflags for size
GCFLAGS=""

echo "Building ${BINARY} ${VERSION} on ${DATE}"
echo

mkdir -p "${DIST}"

for target in "${TARGETS[@]}"; do
    GOOS="${target%%/*}"
    GOARCH="${target##*/}"
    out="${DIST}/${BINARY}-${GOOS}-${GOARCH}"

    printf "  %-20s → %s\n" "${GOOS}/${GOARCH}" "${out}"

    CGO_ENABLED=0 GOOS="${GOOS}" GOARCH="${GOARCH}" \
        go build \
            -trimpath \
            -ldflags "${LDFLAGS}" \
            -gcflags "${GCFLAGS}" \
            -o "${out}" \
            .

    # Further strip with objcopy if targeting native arch and tool is available
    if [[ "${GOARCH}" == "$(uname -m | sed 's/x86_64/amd64/')" ]] && command -v objcopy &>/dev/null; then
        objcopy --strip-all "${out}" "${out}" 2>/dev/null || true
    fi

    size=$(du -sh "${out}" | cut -f1)
    printf "    size: %s\n" "${size}"
done

echo
echo "Done. Artifacts in ./${DIST}/"
ls -lh "${DIST}/"
