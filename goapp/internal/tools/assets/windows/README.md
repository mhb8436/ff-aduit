# 내장 ffmpeg 자산 (Windows)

이 폴더의 `ffmpeg.exe.gz`, `ffprobe.exe.gz` 는 **Windows 단일 exe 빌드 시
exe 안에 내장(`go:embed`)되는 ffmpeg 정적 바이너리**(gzip 압축)입니다.

- 대용량(각 ~35MB)이라 git 에는 커밋하지 않습니다(.gitignore).
- Windows 빌드 전에 아래 명령으로 생성하세요:

```bash
bash goapp/scripts/fetch_win_ffmpeg.sh
```

맥/리눅스 빌드는 이 자산이 없어도 됩니다(시스템 PATH 의 ffmpeg 사용).
