package checks

import (
	"fmt"
	"math"

	"vqc/internal/model"
	"vqc/internal/spec"
)

const catSync = "동기화(싱크)"

// AVSync 는 영상/음성 싱크(동기) 불일치를 점검한다.
// RFP 중점점검(디지털 변환-품질관리): "싱크 불일치".
//
// 판정 근거는 ffprobe 가 보고하는 스트림별 start_time(재생 시작 오프셋)이다.
// 영상 스트림과 오디오 스트림의 시작 오프셋 차이가 크면, 변환/먹싱 과정에서
// A/V 정렬이 어긋난 것으로 재생 시 립싱크 이상이 나타난다. 컨테이너 시작 시점의
// 정적 오프셋을 측정하므로 --deep 없이도 동작한다(전 구간 드리프트는 별도 확인 권장).
func AVSync(fr *model.FileReport, probe *model.Probe, sp *spec.Spec) {
	cfg := spec.Map(sp.Quality(), "av_sync")
	if !spec.Bool(cfg, "enabled", true) {
		return
	}
	v := probe.FirstStream("video")
	a := probe.FirstStream("audio")
	if v == nil || a == nil {
		fr.Add(model.CheckResult{CheckID: "sync.av_offset", Category: catSync,
			Title: "영상·음성 싱크", Severity: model.Skip,
			Message: "영상 또는 오디오 스트림이 없어 싱크 검사를 생략합니다."})
		return
	}

	vStart := model.ParseFloat(v.StartTime)
	aStart := model.ParseFloat(a.StartTime)
	offsetMs := math.Abs(vStart-aStart) * 1000

	warnMs, _ := spec.Float(cfg, "warn_offset_ms", 100)
	failMs, _ := spec.Float(cfg, "fail_offset_ms", 400)

	var sev model.Severity
	var msg string
	switch {
	case offsetMs >= failMs:
		sev = model.Fail
		msg = fmt.Sprintf("영상·음성 싱크 어긋남: 시작 오프셋 %.0fms (한계 %.0fms). 립싱크 이상으로 재변환/재정렬이 필요합니다.", offsetMs, failMs)
	case offsetMs >= warnMs:
		sev = model.Warn
		msg = fmt.Sprintf("영상·음성 싱크 오차 발견: 시작 오프셋 %.0fms (권고 %.0fms). 재생 시 립싱크 확인 권장.", offsetMs, warnMs)
	default:
		sev = model.Pass
		msg = fmt.Sprintf("영상·음성 싱크 양호: 시작 오프셋 %.0fms.", offsetMs)
	}
	fr.Add(model.CheckResult{CheckID: "sync.av_offset", Category: catSync,
		Title: "영상·음성 싱크", Severity: sev, Message: msg,
		Expected: fmt.Sprintf("오프셋 < %.0fms", warnMs),
		Actual:   fmt.Sprintf("%.0fms", offsetMs),
		Evidence: map[string]any{
			"video_start_sec": r3(vStart), "audio_start_sec": r3(aStart),
			"offset_ms": r1(offsetMs)}})
}
