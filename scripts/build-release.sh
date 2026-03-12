#!/usr/bin/env bash
set -euo pipefail

VERSION="${1:-dev}"
COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME=$(date -u '+%Y-%m-%dT%H:%M:%SZ')
LDFLAGS="-s -w -X github.com/ios9000/PGPulse_01/internal/api.Version=${VERSION}"

echo "=== PGPulse Release Build v${VERSION} (${COMMIT}) ==="

# Build frontend
echo "--- Building frontend ---"
cd web
npm ci --silent
npm run build
cd ..

# Build Go binaries
PLATFORMS=(
  "windows/amd64"
  "linux/amd64"
)

DIST_DIR="dist"
rm -rf "${DIST_DIR}"
mkdir -p "${DIST_DIR}"

for platform in "${PLATFORMS[@]}"; do
  IFS='/' read -r goos goarch <<< "${platform}"

  ext=""
  if [ "${goos}" = "windows" ]; then
    ext=".exe"
  fi

  output="${DIST_DIR}/pgpulse-server-${goos}-${goarch}${ext}"

  echo "--- Building ${goos}/${goarch} ---"
  CGO_ENABLED=0 GOOS="${goos}" GOARCH="${goarch}" \
    go build -ldflags "${LDFLAGS}" -o "${output}" ./cmd/pgpulse-server

  echo "  -> ${output} ($(du -h "${output}" | cut -f1))"
done

# Create release archives
echo "--- Creating archives ---"
for platform in "${PLATFORMS[@]}"; do
  IFS='/' read -r goos goarch <<< "${platform}"

  ext=""
  if [ "${goos}" = "windows" ]; then
    ext=".exe"
  fi

  binary="${DIST_DIR}/pgpulse-server-${goos}-${goarch}${ext}"
  archive_name="pgpulse-${VERSION}-${goos}-${goarch}"
  archive_dir="${DIST_DIR}/${archive_name}"

  mkdir -p "${archive_dir}"
  cp "${binary}" "${archive_dir}/pgpulse-server${ext}"
  cp config.sample.yaml "${archive_dir}/" 2>/dev/null || true
  cp README.txt "${archive_dir}/" 2>/dev/null || true

  if [ "${goos}" = "windows" ]; then
    (cd "${DIST_DIR}" && zip -qr "${archive_name}.zip" "${archive_name}/")
    echo "  -> ${DIST_DIR}/${archive_name}.zip"
  else
    tar -czf "${DIST_DIR}/${archive_name}.tar.gz" -C "${DIST_DIR}" "${archive_name}"
    echo "  -> ${DIST_DIR}/${archive_name}.tar.gz"
  fi

  rm -rf "${archive_dir}"
done

echo "=== Build complete ==="
ls -lh "${DIST_DIR}"/*.zip "${DIST_DIR}"/*.tar.gz 2>/dev/null || true
