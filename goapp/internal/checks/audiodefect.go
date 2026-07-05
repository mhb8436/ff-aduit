package checks

import (
	"fmt"
	"regexp"
	"strings"

	"vqc/internal/model"
	"vqc/internal/spec"
	"vqc/internal/tools"
)

const catAudio = "음질"

var (
	reSilEnd = regexp.MustCompile(`silence_end:\s*([\-\d.]+)\s*\|\s*silence_duration:\s*([\d.]+)`)
	reI      = regexp.MustCompile(`I:\s*([\-\d.]+)\s*LUFS`)
	reTP     = regexp.MustCompile(`(?:True peak|Peak):\s*([\-\d.]+)\s*dBFS`)
)

// AudioDefect 는 음질 결함(무음/라우드니스/트루피크)을 검출한다.
func AudioDefect(fr *model.FileReport, probe *model.Probe, sp *spec.Spec, tl tools.Tools, deep bool) {
	if probe.FirstStream("audio") == nil {
		fr.Add(model.CheckResult{CheckID: "audio.defect", Category: catAudio,
			Title: "음질 결함 검출", Severity: model.Skip, Message: "오디오 스트림이 없어 검사 생략."})
		return
	}
	if !deep {
		fr.Add(model.CheckResult{CheckID: "audio.defect", Category: catAudio,
			Title: "음질 결함 검출", Severity: model.Skip, Message: "빠른 검사 모드: 결함 검출 생략(--deep)."})
		return
	}
	qa := spec.Map(sp.Quality(), "audio")
	dur := probe.DurationSec()
	silencedetect(fr, spec.Map(qa, "silencedetect"), dur, tl)
	loudness(fr, spec.Map(qa, "loudness"), tl)
}

func silencedetect(fr *model.FileReport, cfg map[string]any, dur float64, tl tools.Tools) {
	if !spec.Bool(cfg, "enabled", true) {
		return
	}
	noise, _ := spec.Float(cfg, "noise_db", -50)
	minDur, _ := spec.Float(cfg, "silence_min_duration", 3.0)
	af := fmt.Sprintf("silencedetect=n=%gdB:d=%g", noise, minDur)
	log, _ := tl.FilterStderr(fr.Path, "", af)

	edge, _ := spec.Float(cfg, "max_edge_sec", 3.0)
	var segs []map[string]any
	total := 0.0
	for _, m := range reSilEnd.FindAllStringSubmatch(log, -1) {
		end := atof(m[1])
		d := atof(m[2])
		start := end - d
		if start <= edge || (dur > 0 && end >= dur-edge) {
			continue
		}
		segs = append(segs, map[string]any{"start": r2(start), "end": r2(end), "duration": r2(d)})
		total += d
	}
	ratio := 0.0
	if dur > 0 {
		ratio = total / dur
	}
	warnR, _ := spec.Float(cfg, "warn_total_ratio", 0.05)
	failR, _ := spec.Float(cfg, "fail_total_ratio", 0.20)
	var sev model.Severity
	var msg string
	switch {
	case ratio >= failR:
		sev = model.Fail
		msg = fmt.Sprintf("무음 구간 과다: 누적 %.1fs (%.1f%%). 오디오 유실 또는 싱크 이상 의심 — 재변환 검토 필요.", total, ratio*100)
	case ratio >= warnR || len(segs) > 0:
		sev = model.Warn
		msg = fmt.Sprintf("무음 구간 발견: 누적 %.1fs (%.1f%%), %d개 구간. 원본 대조 확인 권장.", total, ratio*100, len(segs))
	default:
		sev = model.Pass
		msg = "본편 내 이상 무음 구간 없음."
	}
	fr.Add(model.CheckResult{CheckID: "audio.silencedetect", Category: catAudio,
		Title: "무음 구간", Severity: sev, Message: msg,
		Expected: fmt.Sprintf("무음 비율 < %.0f%%", warnR*100),
		Actual:   fmt.Sprintf("%.1f%%", ratio*100),
		Evidence: map[string]any{"segments": clip(segs, 20), "total_silence_sec": r2(total)}})
}

func loudness(fr *model.FileReport, cfg map[string]any, tl tools.Tools) {
	if !spec.Bool(cfg, "enabled", true) {
		return
	}
	af := "ebur128=peak=true,astats=metadata=1:reset=0"
	log, _ := tl.FilterStderr(fr.Path, "", af)

	var integrated, truePeak *float64
	if ms := reI.FindAllStringSubmatch(log, -1); len(ms) > 0 {
		v := atof(ms[len(ms)-1][1])
		integrated = &v
	}
	if ms := reTP.FindAllStringSubmatch(log, -1); len(ms) > 0 {
		v := atof(ms[0][1])
		for _, m := range ms {
			if x := atof(m[1]); x > v {
				v = x
			}
		}
		truePeak = &v
	}
	target, _ := spec.Float(cfg, "target_lufs", -23.0)
	tol, _ := spec.Float(cfg, "lufs_tolerance", 6.0)
	maxTP, _ := spec.Float(cfg, "max_true_peak_dbtp", -1.0)

	sev := model.Pass
	var issues []string
	if integrated != nil && absf(*integrated-target) > tol {
		sev = model.Warn
		issues = append(issues, fmt.Sprintf("통합 라우드니스 %.1f LUFS (목표 %g±%g) 이탈", *integrated, target, tol))
	}
	if truePeak != nil && *truePeak > maxTP {
		sev = model.Warn
		issues = append(issues, fmt.Sprintf("트루피크 %.1f dBFS (상한 %g) 초과 — 클리핑 위험", *truePeak, maxTP))
	}
	msg := fmt.Sprintf("오디오 레벨 적정(I=%s LUFS, TP=%s dBFS).", pstr(integrated), pstr(truePeak))
	if len(issues) > 0 {
		msg = "오디오 레벨 점검 필요: " + strings.Join(issues, "; ") + "."
	}
	fr.Add(model.CheckResult{CheckID: "audio.loudness", Category: catAudio,
		Title: "라우드니스/트루피크", Severity: sev, Message: msg,
		Expected: map[string]any{"target_lufs": target, "tolerance": tol, "max_true_peak": maxTP},
		Actual:   map[string]any{"integrated_lufs": integrated, "true_peak_dbfs": truePeak}})
}

func absf(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func pstr(p *float64) string {
	if p == nil {
		return "N/A"
	}
	return fmt.Sprintf("%.1f", *p)
}
