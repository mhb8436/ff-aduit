#!/usr/bin/env bash
# 검증용 샘플 영상 생성 스크립트.
# 정상 파일과 결함(블랙/무음/규격위반) 파일을 만들어 검사 엔진을 시연한다.
set -e
OUT="${1:-samples}"
mkdir -p "$OUT/보존마스터" "$OUT/서비스"

echo "[1] 정상 보존마스터(SD, MPEG-2, 48k PCM, 동영상+톤)"
ffmpeg -y -v error \
  -f lavfi -i "testsrc2=size=720x576:rate=25:duration=8" \
  -f lavfi -i "sine=frequency=1000:sample_rate=48000:duration=8" \
  -c:v mpeg2video -b:v 25M -minrate 20M -maxrate 30M -bufsize 30M \
  -c:a pcm_s16le -ar 48000 -ac 2 \
  "$OUT/보존마스터/KBS_A0001_20260101_MST.mxf"

echo "[2] 정상 서비스본(H.264, AAC)"
ffmpeg -y -v error \
  -f lavfi -i "testsrc2=size=1280x720:rate=30:duration=6" \
  -f lavfi -i "sine=frequency=440:sample_rate=48000:duration=6" \
  -c:v libx264 -pix_fmt yuv420p -b:v 5M -c:a aac -ar 48000 -ac 2 \
  "$OUT/서비스/KBS_A0001_20260101_SVC.mp4"

echo "[3] 결함: 본편 중앙 블랙 구간 + 무음 구간 (보존마스터)"
# 앞8초 동영상+톤, 가운데5초 블랙+무음, 뒤8초 동영상+톤 (총 21초, 블랙 8~13s: 앞뒤 여백 밖)
ffmpeg -y -v error \
  -f lavfi -i "testsrc2=size=720x576:rate=25:duration=8" \
  -f lavfi -i "sine=frequency=1000:sample_rate=48000:duration=8" \
  -f lavfi -i "color=c=black:size=720x576:rate=25:duration=5" \
  -f lavfi -i "anullsrc=sample_rate=48000:channel_layout=stereo:duration=5" \
  -f lavfi -i "testsrc2=size=720x576:rate=25:duration=8" \
  -f lavfi -i "sine=frequency=1000:sample_rate=48000:duration=8" \
  -filter_complex "[0:v][2:v][4:v]concat=n=3:v=1:a=0[v];[1:a][3:a][5:a]concat=n=3:v=0:a=1[a]" \
  -map "[v]" -map "[a]" \
  -c:v mpeg2video -b:v 25M -minrate 20M -maxrate 30M -bufsize 30M \
  -c:a pcm_s16le -ar 48000 -ac 2 \
  "$OUT/보존마스터/KBS_B0002_20260102_MST.mxf"

echo "[4] 규격위반: 서비스 폴더에 과압축 저해상 파일(비트레이트/해상도 위반)"
ffmpeg -y -v error \
  -f lavfi -i "testsrc2=size=320x240:rate=15:duration=5" \
  -f lavfi -i "sine=frequency=440:sample_rate=22050:duration=5" \
  -c:v libx264 -pix_fmt yuv420p -b:v 300k -c:a aac -ar 22050 -ac 1 \
  "$OUT/서비스/badname service file.mp4"

echo "[5] 잘못된 명명규칙 파일(정상 영상이나 파일명 규칙 위반)"
ffmpeg -y -v error \
  -f lavfi -i "testsrc2=size=1280x720:rate=30:duration=4" \
  -f lavfi -i "sine=frequency=440:sample_rate=48000:duration=4" \
  -c:v libx264 -pix_fmt yuv420p -b:v 5M -c:a aac -ar 48000 -ac 2 \
  "$OUT/서비스/영상클립001.mp4"

echo "[6] 결함: 영상·음성 싱크 어긋남(오디오 478ms 지연) — A/V 싱크 FAIL 시연"
ffmpeg -y -v error \
  -f lavfi -i "testsrc2=size=1280x720:rate=30:duration=6" \
  -itsoffset 0.5 -f lavfi -i "sine=frequency=440:sample_rate=48000:duration=6" \
  -map 0:v -map 1:a -c:v libx264 -pix_fmt yuv420p -b:v 5M -c:a aac -ar 48000 -ac 2 \
  "$OUT/서비스/KBS_D0004_20260104_SVC.mp4"

echo "[7] 메타데이터 CSV(--metadata 대조용). 일부 오류를 일부러 넣어 검출을 시연"
# 오류 시연: B0002 해상도 오기(1920x1080↔실측 720x576), A0001_SVC 제목 누락,
#            영상클립001 방송일자 형식 오류(2026-13-05), D0004 코덱 오기(MPEG-2↔실측 h264)
cat > "$OUT/메타데이터.csv" <<'CSV'
파일명,제목,방송사명,방송일자,해상도,코덱,컨테이너,재생시간
KBS_A0001_20260101_MST.mxf,뉴스광장 오프닝,KBS,20260101,720x576,MPEG-2,mxf,00:00:08
KBS_B0002_20260102_MST.mxf,아침마당 1부,KBS,20260102,1920x1080,MPEG-2,mxf,00:00:21
KBS_A0001_20260101_SVC.mp4,,KBS,20260101,1280x720,H.264,mp4,00:00:06
영상클립001.mp4,자료영상 001,KBS,2026-13-05,1280x720,H.264,mp4,00:00:04
KBS_D0004_20260104_SVC.mp4,단신 뉴스,KBS,20260104,1280x720,MPEG-2,mp4,00:00:06
CSV

echo "완료: $OUT"
find "$OUT" -type f
