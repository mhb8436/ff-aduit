//go:build windows

// Windows 빌드에서만 컴파일된다. ffmpeg/ffprobe 정적 바이너리(gzip)를
// exe 안에 내장하고, 최초 실행 시 임시폴더에 추출하여 사용한다.
// → 대상 PC에 ffmpeg 설치 없이 "단일 exe 하나"로 동작한다.
package tools

import (
	"bytes"
	"compress/gzip"
	"embed"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

//go:embed assets/windows/ffmpeg.exe.gz assets/windows/ffprobe.exe.gz
var winAssets embed.FS

// extractEmbedded 는 내장된 gzip 바이너리를 임시폴더에 풀어 경로를 반환한다.
// 이미 추출돼 있으면 재사용한다.
func extractEmbedded(name string) (string, bool, error) {
	gzName := "assets/windows/" + name + ".exe.gz"
	data, err := winAssets.ReadFile(gzName)
	if err != nil {
		return "", false, nil // 내장본 없음 → 상위에서 PATH 폴백
	}
	dir := filepath.Join(os.TempDir(), "vqc-ffmpeg")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", false, err
	}
	dst := filepath.Join(dir, name+".exe")

	// 압축 해제(스트리밍)
	gr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return "", false, err
	}
	defer gr.Close()
	raw, err := io.ReadAll(gr)
	if err != nil {
		return "", false, err
	}

	// 이미 동일 크기로 추출돼 있으면 건너뜀
	if fi, statErr := os.Stat(dst); statErr == nil && fi.Size() == int64(len(raw)) {
		return dst, true, nil
	}
	if err := os.WriteFile(dst, raw, 0o755); err != nil {
		return "", false, fmt.Errorf("내장 %s 추출 실패: %w", name, err)
	}
	return dst, true, nil
}
