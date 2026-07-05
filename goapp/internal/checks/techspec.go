package checks

import (
	"fmt"
	"math"
	"strings"

	"vqc/internal/model"
	"vqc/internal/spec"
	"vqc/internal/tools"
)

const catTech = "기술규격"

// TechSpec 은 파일 규격(컨테이너/코덱/해상도/FPS/스캔/비트레이트/오디오)을
// 프로파일(납품규격) 대비 점검한다.
func TechSpec(fr *model.FileReport, probe *model.Probe, profile map[string]any,
	sp *spec.Spec, tl tools.Tools) {
	v := probe.FirstStream("video")
	a := probe.FirstStream("audio")
	vspec := spec.Map(profile, "video")
	aspec := spec.Map(profile, "audio")

	// --- 컨테이너 ---
	container := probe.Format.FormatName
	allowedCont := spec.StrList(profile, "container")
	if len(allowedCont) > 0 {
		parts := strings.Split(strings.ToLower(container), ",")
		ok := false
		for _, p := range parts {
			if inList(strings.TrimSpace(p), allowedCont) {
				ok = true
				break
			}
		}
		fr.Add(allowResult("techspec.container", "컨테이너 포맷", container, allowedCont, ok, ""))
	}

	// --- 영상 ---
	if v != nil {
		fr.Add(allowStr("techspec.video.codec", "영상 코덱", v.CodecName, spec.StrList(vspec, "codec")))
		fr.Add(rangeResult("techspec.video.width", "가로 해상도", float64(v.Width), spec.Map(vspec, "width"), "px"))
		fr.Add(rangeResult("techspec.video.height", "세로 해상도", float64(v.Height), spec.Map(vspec, "height"), "px"))

		// 프레임레이트(근사 매칭)
		fr2 := v.AvgFrameRate
		if fr2 == "" || fr2 == "0/0" {
			fr2 = v.RFrameRate
		}
		rate := model.ParseRate(fr2)
		allowedFR := spec.FloatList(vspec, "frame_rate")
		if len(allowedFR) > 0 {
			ok := matchRate(rate, allowedFR)
			sev := model.Fail
			msg := fmt.Sprintf("프레임레이트 규격 위반: %gfps (허용 %v).", rate, allowedFR)
			if ok {
				sev, msg = model.Pass, fmt.Sprintf("프레임레이트 적합: %gfps", rate)
			}
			fr.Add(model.CheckResult{CheckID: "techspec.video.frame_rate", Category: catTech,
				Title: "프레임레이트", Severity: sev, Message: msg, Expected: allowedFR, Actual: rate})
		}

		// 스캔 방식
		allowedScan := spec.StrList(vspec, "scan")
		if len(allowedScan) > 0 {
			scan := scanType(v.FieldOrder)
			ok := inList(scan, allowedScan)
			sev := model.Fail
			if ok {
				sev = model.Pass
			} else if scan == "unknown" {
				sev = model.Warn
			}
			msg := fmt.Sprintf("스캔 방식 적합: %s", scan)
			if !ok {
				msg = fmt.Sprintf("스캔 방식 확인 필요/위반: %s (허용 %v). 서비스본은 디인터레이스가 필요합니다.", scan, allowedScan)
			}
			fr.Add(model.CheckResult{CheckID: "techspec.video.scan", Category: catTech,
				Title: "스캔 방식", Severity: sev, Message: msg, Expected: allowedScan, Actual: scan,
				Evidence: map[string]any{"field_order": v.FieldOrder}})
		}

		// 픽셀 포맷
		if pf := spec.StrList(vspec, "pixel_format"); len(pf) > 0 {
			fr.Add(allowStr("techspec.video.pix_fmt", "픽셀 포맷", v.PixFmt, pf))
		}

		checkBitrate(fr, probe, v, vspec)
	} else {
		fr.Add(model.CheckResult{CheckID: "techspec.video", Category: catTech,
			Title: "영상 스트림", Severity: model.Fail,
			Message: "영상 스트림이 없어 영상 규격을 검사할 수 없습니다."})
	}

	// --- 오디오 ---
	if a != nil {
		fr.Add(allowStr("techspec.audio.codec", "오디오 코덱", a.CodecName, spec.StrList(aspec, "codec")))
		if allowedCh := spec.IntList(aspec, "channels"); len(allowedCh) > 0 {
			ok := intInList(a.Channels, allowedCh)
			sev := model.Warn
			msg := fmt.Sprintf("채널수 확인 필요: %dch (허용 %v).", a.Channels, allowedCh)
			if ok {
				sev, msg = model.Pass, fmt.Sprintf("채널수 적합: %dch", a.Channels)
			}
			fr.Add(model.CheckResult{CheckID: "techspec.audio.channels", Category: catTech,
				Title: "오디오 채널수", Severity: sev, Message: msg, Expected: allowedCh, Actual: a.Channels})
		}
		if minSR := spec.Int(aspec, "min_sample_rate", 0); minSR > 0 {
			sr := model.ParseInt(a.SampleRate)
			ok := sr >= minSR
			sev := model.Fail
			msg := fmt.Sprintf("샘플레이트 미달: %dHz (최소 %dHz).", sr, minSR)
			if ok {
				sev, msg = model.Pass, fmt.Sprintf("샘플레이트 적합: %dHz", sr)
			}
			fr.Add(model.CheckResult{CheckID: "techspec.audio.sample_rate", Category: catTech,
				Title: "샘플레이트", Severity: sev, Message: msg,
				Expected: fmt.Sprintf(">= %d", minSR), Actual: sr})
		}
	} else {
		minCh := spec.Int(aspec, "min_channels", 0)
		sev := model.Skip
		msg := "오디오 규격 미지정."
		if minCh > 0 {
			sev = model.Warn
			msg = "오디오 스트림이 없습니다. 무음 원본 여부 확인이 필요합니다."
		}
		fr.Add(model.CheckResult{CheckID: "techspec.audio", Category: catTech,
			Title: "오디오 스트림", Severity: sev, Message: msg})
	}
}

func checkBitrate(fr *model.FileReport, probe *model.Probe, v *model.Stream, vspec map[string]any) {
	minMbps, hasMin := spec.Float(vspec, "min_bitrate_mbps", 0)
	maxMbps, hasMax := spec.Float(vspec, "max_bitrate_mbps", 0)
	if !hasMin && !hasMax {
		return
	}
	br := model.ParseInt(v.BitRate)
	if br == 0 {
		br = model.ParseInt(probe.Format.BitRate)
	}
	exp := map[string]any{"min": minMbps, "max": maxMbps}
	if br == 0 {
		fr.Add(model.CheckResult{CheckID: "techspec.video.bitrate", Category: catTech,
			Title: "영상 비트레이트", Severity: model.Warn,
			Message:  "비트레이트를 추출하지 못했습니다(가변비트레이트 등). 수동 확인 권장.",
			Expected: exp})
		return
	}
	mbps := math.Round(float64(br)/1e6*100) / 100
	switch {
	case hasMin && minMbps > 0 && mbps < minMbps:
		fr.Add(model.CheckResult{CheckID: "techspec.video.bitrate", Category: catTech,
			Title: "영상 비트레이트", Severity: model.Fail,
			Message:  fmt.Sprintf("비트레이트 과소: %gMbps (최소 %gMbps). 과압축으로 화질 열화 우려.", mbps, minMbps),
			Expected: fmt.Sprintf(">= %gMbps", minMbps), Actual: fmt.Sprintf("%gMbps", mbps)})
	case hasMax && maxMbps > 0 && mbps > maxMbps:
		fr.Add(model.CheckResult{CheckID: "techspec.video.bitrate", Category: catTech,
			Title: "영상 비트레이트", Severity: model.Warn,
			Message:  fmt.Sprintf("비트레이트 과다: %gMbps (권장 최대 %gMbps). 저장효율 검토 권장.", mbps, maxMbps),
			Expected: fmt.Sprintf("<= %gMbps", maxMbps), Actual: fmt.Sprintf("%gMbps", mbps)})
	default:
		fr.Add(model.CheckResult{CheckID: "techspec.video.bitrate", Category: catTech,
			Title: "영상 비트레이트", Severity: model.Pass,
			Message: fmt.Sprintf("비트레이트 적합: %gMbps.", mbps), Expected: exp,
			Actual: fmt.Sprintf("%gMbps", mbps)})
	}
}

// ---- 공용 판정 헬퍼 ----

func allowStr(id, title, value string, allowed []string) model.CheckResult {
	if len(allowed) == 0 {
		return model.CheckResult{CheckID: id, Category: catTech, Title: title,
			Severity: model.Skip, Message: "규격 미지정(검사 생략).", Actual: value}
	}
	return allowResult(id, title, value, allowed, inList(strings.ToLower(value), lowerAll(allowed)), "")
}

func allowResult(id, title, value string, allowed []string, ok bool, unit string) model.CheckResult {
	sev := model.Fail
	msg := fmt.Sprintf("%s 규격 위반: 실측 '%s%s' 은(는) 허용값 %v 에 없습니다. 납품규격 미준수로 시정이 필요합니다.", title, value, unit, allowed)
	if ok {
		sev, msg = model.Pass, fmt.Sprintf("%s 적합: %s%s", title, value, unit)
	}
	return model.CheckResult{CheckID: id, Category: catTech, Title: title,
		Severity: sev, Message: msg, Expected: allowed, Actual: value}
}

func rangeResult(id, title string, value float64, rng map[string]any, unit string) model.CheckResult {
	if len(rng) == 0 {
		return model.CheckResult{CheckID: id, Category: catTech, Title: title,
			Severity: model.Skip, Message: "규격 미지정(검사 생략).", Actual: value}
	}
	lo, hasLo := spec.Float(rng, "min", 0)
	hi, hasHi := spec.Float(rng, "max", 0)
	if (hasLo && value < lo) || (hasHi && value > hi) {
		return model.CheckResult{CheckID: id, Category: catTech, Title: title,
			Severity: model.Fail,
			Message:  fmt.Sprintf("%s 범위 위반: 실측 %g%s (허용 %g~%g%s).", title, value, unit, lo, hi, unit),
			Expected: rng, Actual: value}
	}
	return model.CheckResult{CheckID: id, Category: catTech, Title: title,
		Severity: model.Pass,
		Message:  fmt.Sprintf("%s 적합: %g%s (허용 %g~%g%s).", title, value, unit, lo, hi, unit),
		Expected: rng, Actual: value}
}

func matchRate(value float64, allowed []float64) bool {
	for _, a := range allowed {
		if math.Abs(value-a) <= 0.05 {
			return true
		}
	}
	return false
}

func scanType(fieldOrder string) string {
	if fieldOrder == "" || fieldOrder == "unknown" {
		return "unknown"
	}
	if fieldOrder == "progressive" {
		return "progressive"
	}
	return "interlaced"
}

func inList(v string, list []string) bool {
	for _, e := range list {
		if strings.EqualFold(v, e) {
			return true
		}
	}
	return false
}

func intInList(v int, list []int) bool {
	for _, e := range list {
		if v == e {
			return true
		}
	}
	return false
}

func lowerAll(list []string) []string {
	out := make([]string, len(list))
	for i, s := range list {
		out[i] = strings.ToLower(s)
	}
	return out
}
