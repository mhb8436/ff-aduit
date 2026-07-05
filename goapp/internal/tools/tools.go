// Package tools 는 외부 바이너리(ffprobe/ffmpeg) 실행을 캡슐화한다.
//
// 배포 형태에 따라 도구를 다음 우선순위로 해석한다.
//
//	① 내장(embed) 바이너리  — windows 빌드 시 exe 안에 포장된 ffmpeg를 임시폴더에 추출
//	② 실행파일 옆 vendor/   — 수동 동봉
//	③ PATH                 — 시스템 설치본(맥/리눅스 개발·검증)
//
// 이로써 windows 는 "단일 exe 하나"로 무설치 실행되고,
// 맥/리눅스는 시스템 ffmpeg 로 동일 로직을 테스트할 수 있다.
package tools

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"vqc/internal/model"
)

// Tools 는 해석된 ffprobe/ffmpeg 실행 경로를 보관한다.
type Tools struct {
	FFprobe string
	FFmpeg  string
}

var (
	resolveOnce sync.Once
	resolved    Tools
	resolveErr  error
)

// Default 는 도구를 1회 해석하여 반환한다(내장 추출 포함).
func Default() (Tools, error) {
	resolveOnce.Do(func() {
		fp, err := resolve("ffprobe")
		if err != nil {
			resolveErr = err
			return
		}
		fm, err := resolve("ffmpeg")
		if err != nil {
			resolveErr = err
			return
		}
		resolved = Tools{FFprobe: fp, FFmpeg: fm}
	})
	return resolved, resolveErr
}

func exeName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

// resolve 는 도구 1개의 실행 경로를 결정한다.
func resolve(name string) (string, error) {
	// ① 내장 바이너리(빌드에 포함된 경우) 추출
	if p, ok, err := extractEmbedded(name); err != nil {
		return "", err
	} else if ok {
		return p, nil
	}
	// ② 실행파일 옆 vendor/
	if self, err := os.Executable(); err == nil {
		dir := filepath.Dir(self)
		for _, cand := range []string{
			filepath.Join(dir, exeName(name)),
			filepath.Join(dir, "vendor", exeName(name)),
		} {
			if isExec(cand) {
				return cand, nil
			}
		}
	}
	// ③ PATH
	if p, err := exec.LookPath(name); err == nil {
		return p, nil
	}
	return "", fmt.Errorf("%s 를 찾을 수 없습니다. 내장 빌드가 아니면 ffmpeg 설치 후 PATH를 확인하세요", name)
}

func isExec(path string) bool {
	fi, err := os.Stat(path)
	if err != nil || fi.IsDir() {
		return false
	}
	return true
}

// RunProbeJSON 은 ffprobe 로 메타데이터를 JSON 으로 받아 파싱한다.
func (t Tools) RunProbeJSON(path string) (*model.Probe, error) {
	out, err := runCapture(t.FFprobe, "-v", "error",
		"-show_format", "-show_streams", "-of", "json", path)
	if err != nil {
		return nil, fmt.Errorf("ffprobe 실패(%s): %v", filepath.Base(path), err)
	}
	return parseProbe(out)
}

// FilterStderr 는 -vf/-af 필터를 걸어 전체 디코드 후 stderr(필터 로그)를 반환.
// blackdetect/silencedetect/freezedetect 등이 stderr 로 결과를 낸다.
func (t Tools) FilterStderr(path, vf, af string, extra ...string) (string, error) {
	args := []string{"-hide_banner", "-nostats", "-i", path}
	if vf != "" {
		args = append(args, "-vf", vf)
	}
	if af != "" {
		args = append(args, "-af", af)
	}
	args = append(args, extra...)
	args = append(args, "-f", "null", nullSink())
	_, stderr, _ := runFull(t.FFmpeg, args...)
	return stderr, nil
}

// MetadataPrint 은 metadata=print(file=-) 로 프레임 통계를 stdout 으로 받는다.
func (t Tools) MetadataPrint(path, vf string) (string, error) {
	stdout, _, err := runFull(t.FFmpeg, "-hide_banner", "-nostats", "-v", "error",
		"-i", path, "-vf", vf, "-an", "-f", "null", nullSink())
	if err != nil {
		// metadata print 는 stdout 으로 나오므로 종료코드와 무관하게 stdout 사용
		return stdout, nil
	}
	return stdout, nil
}

// DecodeErrors 는 전체 디코드하여 오류 라인 수를 센다(재생가능/무결성).
func (t Tools) DecodeErrors(path string) (int, string) {
	_, stderr, _ := runFull(t.FFmpeg, "-hide_banner", "-nostats",
		"-xerror", "-v", "error", "-i", path, "-f", "null", nullSink())
	n := 0
	for _, ln := range strings.Split(stderr, "\n") {
		if strings.TrimSpace(ln) != "" {
			n++
		}
	}
	if len(stderr) > 2000 {
		stderr = stderr[:2000]
	}
	return n, stderr
}

func nullSink() string {
	if runtime.GOOS == "windows" {
		return "NUL"
	}
	return os.DevNull
}
