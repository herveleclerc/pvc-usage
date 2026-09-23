#!/usr/bin/env bash
set -euo pipefail

VERSION="${1:-v0.1.0}"
CHECKSUMS_FILE="dist/releases/checksums.txt"
OUTPUT_FILE="krew.yaml"

if [ ! -f "${CHECKSUMS_FILE}" ]; then
  echo "Error: ${CHECKSUMS_FILE} not found. Run ./scripts/package.sh first."
  exit 1
fi

get_sha() {
  local pattern="$1"
  grep "${pattern}" "${CHECKSUMS_FILE}" | awk '{print $1}'
}

DARWIN_AMD64_SHA=$(get_sha "darwin_amd64")
DARWIN_ARM64_SHA=$(get_sha "darwin_arm64")
LINUX_AMD64_SHA=$(get_sha "linux_amd64")
LINUX_ARM64_SHA=$(get_sha "linux_arm64")
WINDOWS_AMD64_SHA=$(get_sha "windows_amd64")

cat <<EOF > "${OUTPUT_FILE}"
apiVersion: krew.googlecontainertools.github.com/v1alpha2
kind: Plugin
metadata:
  name: pvc-usage
spec:
  version: "${VERSION}"
  homepage: https://github.com/herveleclerc/pvc-usage
  shortDescription: Identify unused PVCs and reclaim orphaned storage in Kubernetes 1.37+
  description: |
    kubectl-pvc-usage is a kubectl plugin designed to detect unused PersistentVolumeClaims.
    It natively leverages the Kubernetes 1.37 Beta feature PersistentVolumeClaimUnusedSinceTime
    (KEP-5541) to identify orphaned volumes, compute idle durations, and provide FinOps
    storage reclamation insights.
  caveats: |
    Requires Kubernetes 1.37+ (or 1.36 with feature gate PersistentVolumeClaimUnusedSinceTime enabled)
    for native Unused condition tracking.
  platforms:
  - selector:
      matchLabels:
        os: darwin
        arch: amd64
    uri: https://github.com/herveleclerc/pvc-usage/releases/download/${VERSION}/kubectl-pvc-usage_${VERSION}_darwin_amd64.tar.gz
    sha256: ${DARWIN_AMD64_SHA}
    bin: kubectl-pvc-usage
  - selector:
      matchLabels:
        os: darwin
        arch: arm64
    uri: https://github.com/herveleclerc/pvc-usage/releases/download/${VERSION}/kubectl-pvc-usage_${VERSION}_darwin_arm64.tar.gz
    sha256: ${DARWIN_ARM64_SHA}
    bin: kubectl-pvc-usage
  - selector:
      matchLabels:
        os: linux
        arch: amd64
    uri: https://github.com/herveleclerc/pvc-usage/releases/download/${VERSION}/kubectl-pvc-usage_${VERSION}_linux_amd64.tar.gz
    sha256: ${LINUX_AMD64_SHA}
    bin: kubectl-pvc-usage
  - selector:
      matchLabels:
        os: linux
        arch: arm64
    uri: https://github.com/herveleclerc/pvc-usage/releases/download/${VERSION}/kubectl-pvc-usage_${VERSION}_linux_arm64.tar.gz
    sha256: ${LINUX_ARM64_SHA}
    bin: kubectl-pvc-usage
  - selector:
      matchLabels:
        os: windows
        arch: amd64
    uri: https://github.com/herveleclerc/pvc-usage/releases/download/${VERSION}/kubectl-pvc-usage_${VERSION}_windows_amd64.zip
    sha256: ${WINDOWS_AMD64_SHA}
    bin: kubectl-pvc-usage.exe
EOF

echo "Updated ${OUTPUT_FILE} with SHA256 checksums for ${VERSION}"
