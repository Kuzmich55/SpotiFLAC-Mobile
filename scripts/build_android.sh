#!/usr/bin/env bash
# Build the supported Android release APKs for SpotiFLAC Mobile.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
OUTPUT_DIR="$PROJECT_DIR/build/app/outputs/flutter-apk"

cd "$PROJECT_DIR"
BUILD_GIT_COMMIT="$(git rev-parse --short=8 HEAD)"
flutter build apk \
  --release \
  --split-per-abi \
  --target-platform android-arm,android-arm64 \
  --dart-define="GIT_COMMIT=$BUILD_GIT_COMMIT" \
  "$@"

for apk in app-armeabi-v7a-release.apk app-arm64-v8a-release.apk; do
  if [[ ! -f "$OUTPUT_DIR/$apk" ]]; then
    echo "Error: expected APK was not created: $OUTPUT_DIR/$apk" >&2
    exit 1
  fi
done

echo "Built Android release APKs in $OUTPUT_DIR"
