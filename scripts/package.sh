#!/usr/bin/env bash
set -euo pipefail

VERSION="${1:-v0.1.0}"
DIST_DIR="dist"
RELEASES_DIR="${DIST_DIR}/releases"

echo "Packaging release ${VERSION}..."
rm -rf "${RELEASES_DIR}"
mkdir -p "${RELEASES_DIR}"

PLATFORMS=(
  "linux/amd64"
  "linux/arm64"
  "darwin/amd64"
  "darwin/arm64"
  "windows/amd64"
)

for platform in "${PLATFORMS[@]}"; do
  OS="${platform%/*}"
  ARCH="${platform#*/}"
  STAGE_DIR="${DIST_DIR}/stage_${OS}_${ARCH}"
  rm -rf "${STAGE_DIR}"
  mkdir -p "${STAGE_DIR}"

  SRC_BIN="${DIST_DIR}/kubectl-pvc-usage-${OS}-${ARCH}"
  if [ "${OS}" = "windows" ]; then
    SRC_BIN="${SRC_BIN}.exe"
    cp "${SRC_BIN}" "${STAGE_DIR}/kubectl-pvc-usage.exe"
    cp LICENSE README.md "${STAGE_DIR}/"
    ARCHIVE_NAME="kubectl-pvc-usage_${VERSION}_${OS}_${ARCH}.zip"
    (cd "${STAGE_DIR}" && zip -q -r "../../${RELEASES_DIR}/${ARCHIVE_NAME}" .)
  else
    cp "${SRC_BIN}" "${STAGE_DIR}/kubectl-pvc-usage"
    cp LICENSE README.md "${STAGE_DIR}/"
    ARCHIVE_NAME="kubectl-pvc-usage_${VERSION}_${OS}_${ARCH}.tar.gz"
    (cd "${STAGE_DIR}" && tar -czf "../../${RELEASES_DIR}/${ARCHIVE_NAME}" .)
  fi

  rm -rf "${STAGE_DIR}"
  echo "Created ${ARCHIVE_NAME}"
done

echo "Generating SHA256 checksums..."
cd "${RELEASES_DIR}"
shasum -a 256 kubectl-pvc-usage_* > checksums.txt
cd ../..

echo "Release artifacts ready in ${RELEASES_DIR}:"
cat "${RELEASES_DIR}/checksums.txt"

