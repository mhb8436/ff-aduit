#!/usr/bin/env bash
# vqc 전 플랫폼 빌드 — 맥 한 대에서 win/mac/linux 실행파일을 한 번에 생성.
#
#   Windows 빌드는 ffmpeg 내장 자산(assets/windows/*.exe.gz)이 필요하다.
#   없으면 scripts/fetch_win_ffmpeg.sh 를 먼저 실행하라는 안내를 출력한다.
set -e
cd "$(dirname "$0")"
OUT="${1:-dist}"
mkdir -p "$OUT"

echo "== vqc 빌드 =="

# macOS (개발·검증용, 시스템 PATH ffmpeg 사용)
echo "[mac/arm64]  → $OUT/vqc-mac-arm64"
GOOS=darwin GOARCH=arm64 go build -o "$OUT/vqc-mac-arm64" .

# Linux (검수환경이 리눅스인 경우, 시스템 PATH ffmpeg 사용)
echo "[linux/amd64] → $OUT/vqc-linux-amd64"
GOOS=linux GOARCH=amd64 go build -o "$OUT/vqc-linux-amd64" .

# Windows (단일 exe, ffmpeg 내장)
if [ -f internal/tools/assets/windows/ffmpeg.exe.gz ]; then
  echo "[windows/amd64] → $OUT/vqc.exe (ffmpeg 내장)"
  GOOS=windows GOARCH=amd64 go build -o "$OUT/vqc.exe" .
else
  echo "[windows/amd64] 건너뜀 — ffmpeg 내장 자산 없음."
  echo "  → 먼저 실행: bash scripts/fetch_win_ffmpeg.sh"
fi

echo "== 완료 =="
ls -lh "$OUT"
