// Package checks 는 항목별 영상 검사 로직을 담는다.
// 각 함수는 FileReport 에 CheckResult 를 추가한다.
package checks

import (
	"fmt"

	"vqc/internal/model"
	"vqc/internal/spec"
	"vqc/internal/tools"
)

const catIntegrity = "무결성/재생가능"

// Integrity 는 재생 가능 여부(무결성)를 점검한다.
// RFP 중점점검: "재생 가능 여부", "저장매체 이상 여부".
func Integrity(fr *model.FileReport, probe *model.Probe, sp *spec.Spec, tl tools.Tools, deep bool) {
	q := spec.Map(sp.Quality(), "integrity")
	maxErr := spec.Int(q, "max_decode_errors", 0)
	warnErr := spec.Int(q, "warn_decode_errors", 0)

	// 1) 영상 스트림 존재
	hasVideo := probe.FirstStream("video") != nil
	if !hasVideo {
		fr.Add(model.CheckResult{
			CheckID: "integrity.has_video_stream", Category: catIntegrity,
			Title: "영상 스트림 존재", Severity: model.Fail,
			Message:  "영상 스트림이 없습니다. 변환 산출물이 손상되었거나 오디오 전용 파일입니다.",
			Expected: "video stream >= 1", Actual: 0,
		})
	} else {
		fr.Add(model.CheckResult{
			CheckID: "integrity.has_video_stream", Category: catIntegrity,
			Title: "영상 스트림 존재", Severity: model.Pass,
			Message: "영상 스트림 확인.", Expected: "video stream >= 1", Actual: 1,
		})
	}

	// 2) 전체 디코드 무결성 (deep 에서만)
	if !deep {
		fr.Add(model.CheckResult{
			CheckID: "integrity.decode", Category: catIntegrity,
			Title: "전체 디코드 무결성", Severity: model.Skip,
			Message: "빠른 검사 모드: 전체 디코드 생략(--deep 로 활성화).",
		})
		return
	}

	nErr, stderr := tl.DecodeErrors(fr.Path)
	var sev model.Severity
	var msg string
	switch {
	case nErr > maxErr:
		sev = model.Fail
		msg = fmt.Sprintf("디코드 중 오류 %d건 발생. 파일 손상/불완전 변환으로 재생 이상이 우려됩니다. 재변환 또는 원본 재점검이 필요합니다.", nErr)
	case nErr > warnErr:
		sev = model.Warn
		msg = fmt.Sprintf("디코드 경고 %d건. 재생에는 지장이 없으나 확인이 필요합니다.", nErr)
	default:
		sev = model.Pass
		msg = "전체 구간 디코드 정상. 재생 가능."
	}
	ev := map[string]any{"decode_errors": nErr}
	if nErr > 0 {
		if len(stderr) > 500 {
			stderr = stderr[:500]
		}
		ev["stderr_head"] = stderr
	}
	fr.Add(model.CheckResult{
		CheckID: "integrity.decode", Category: catIntegrity,
		Title: "전체 디코드 무결성", Severity: sev, Message: msg,
		Expected: fmt.Sprintf("오류 <= %d", maxErr),
		Actual:   fmt.Sprintf("오류 %d건", nErr), Evidence: ev,
	})
}
