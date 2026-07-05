#!/usr/bin/env bash
# Windows용 ffmpeg/ffprobe 정적 바이너리를 내려받아 내장 자산으로 준비한다.
# go:embed 대상(assets/windows/*.exe.gz)을 생성한다.
#   - Windows exe 크로스빌드 전에 1회 실행.
#   - 용량 절감을 위해 gzip 압축 상태로 내장한다(실행 시 자동 해제).
set -e
cd "$(dirname "$0")/.."
ASSETS="internal/tools/assets/windows"
URL="https://www.gyan.dev/ffmpeg/builds/ffmpeg-release-essentials.zip"
TMP="$(mktemp -d)"
mkdir -p "$ASSETS"

echo "[1/3] 다운로드: $URL"
curl -L --fail -o "$TMP/ffmpeg.zip" "$URL"

echo "[2/3] 압축 해제"
unzip -q -o "$TMP/ffmpeg.zip" -d "$TMP"
BIN="$(find "$TMP" -maxdepth 2 -type d -name bin | head -1)"

echo "[3/3] gzip 내장 자산 생성"
gzip -6 -c "$BIN/ffmpeg.exe"  > "$ASSETS/ffmpeg.exe.gz"
gzip -6 -c "$BIN/ffprobe.exe" > "$ASSETS/ffprobe.exe.gz"
rm -rf "$TMP"

ls -lh "$ASSETS"
echo "완료. 이제 build_all.sh 또는 GOOS=windows go build 로 vqc.exe 를 만들 수 있습니다."
