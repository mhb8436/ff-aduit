# vqc — 단일 실행파일 영상 검사 감리 도구 (Go)

방송자료 디지털화 영상 검사 엔진입니다. 폐쇄망 감리 검수환경에 **단일 실행파일 하나만
반입**하면 되도록 배포에 최적화된 Go 구현입니다.

## 배포 이점

| | 내용 |
|---|---|
| **크로스 컴파일** | 맥 한 대에서 win/mac/linux 실행파일을 한 번에 생성 |
| **단일 파일 반입** | `vqc.exe` **하나**에 ffmpeg 까지 내장 → 폐쇄망에 파일 1개만 반입 |
| **무설치** | 대상 PC에 런타임·ffmpeg 설치 불필요 |
| **백신 오탐** | 네이티브 바이너리라 인터프리터 번들 exe 대비 오탐 적음 |

Windows 빌드는 ffmpeg/ffprobe 정적 바이너리를 **exe 안에 gzip 내장**하고,
최초 실행 시 `%TEMP%\vqc-ffmpeg\` 에 자동 추출하여 사용합니다(`internal/tools`).
맥/리눅스 빌드는 내장 없이 **시스템 PATH 의 ffmpeg** 를 사용하므로, 개발 PC에서
동일 로직을 그대로 테스트할 수 있습니다.

## 빌드

```bash
# (Windows exe 를 만들 때만) ffmpeg 내장 자산 준비 — 1회
bash scripts/fetch_win_ffmpeg.sh

# 전 플랫폼 빌드 → dist/
bash build_all.sh
#   dist/vqc.exe          (76MB, ffmpeg 내장, 단일 파일)
#   dist/vqc-mac-arm64    (4MB, 시스템 ffmpeg 사용)
#   dist/vqc-linux-amd64  (5MB, 시스템 ffmpeg 사용)

# 개발 중 맥에서 바로 실행/테스트 (ffmpeg 는 brew 등으로 설치돼 있어야 함)
go run . inspect ../samples --deep --no-sample --inventory ../samples/납품목록.csv --report out
```

> Windows exe 만 단일 파일이고, 맥/리눅스 빌드는 시스템 ffmpeg 를 사용합니다.
> 맥/리눅스도 단일 파일로 만들려면 각 OS 용 ffmpeg 정적 바이너리를 같은 방식으로 내장하면 됩니다.

## 사용법

```bash
vqc inspect <폴더|파일> --deep --inventory 납품목록.csv --report out   # 검사+리포트
vqc inspect <폴더> --no-sample                                        # 전수검사
vqc plan <폴더>                                                       # 표본 선정 미리보기
vqc probe <파일>                                                      # 기술 메타데이터
vqc --version
```

검사 기준(`spec.default.yaml`)은 exe 에 **내장**되며, `--spec 파일.yaml` 로 외부 기준을
지정하면 그것이 우선합니다. 사업 납품규격이 확정되면 이 YAML 만 교체합니다.

## 구조

```
goapp/
  main.go                  CLI (inspect / plan / probe)
  internal/
    engine/     수집→표본→검사→목록대조 오케스트레이션
    tools/      ffprobe/ffmpeg 실행 + ffmpeg 내장·추출(build tag 분리)
      embed_windows.go   //go:build windows — ffmpeg 내장
      embed_other.go     //go:build !windows — PATH 사용
      assets/windows/    내장용 gz(빌드 시 생성, git 제외)
    spec/       검사기준 YAML 로더 + 프로파일 라우팅 (spec.default.yaml 내장)
    checks/     integrity/techspec/videodefect/audiodefect/filenaming/inventory
    sampling/   Cochran + 층화 표본
    report/     console/HTML/CSV/JSON
    model/      결과 데이터 모델
  build_all.sh             전 플랫폼 빌드
  scripts/fetch_win_ffmpeg.sh   내장용 ffmpeg 다운로드
```

## 검증 상태 (현재 저장소)

- 맥 빌드로 `samples` 검사 → 결함 검출 정상(블랙 23.8%·규격위반·납품목록 지적)
- 맥에서 windows exe 크로스빌드 → **76MB 단일 PE32+ exe** 생성
- 내장 gz 자산이 원본 ffmpeg 와 **바이트 일치** 확인(추출 파이프라인 정합)
- ※ windows exe 의 **최종 실행 스모크테스트는 Windows PC 에서 1회** 수행 권장(맥에는 wine 미설치)
