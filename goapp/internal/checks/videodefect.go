package checks

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"vqc/internal/model"
	"vqc/internal/spec"
	"vqc/internal/tools"
)

const catVideo = "화질"

var (
	reBlack   = regexp.MustCompile(`black_start:([\d.]+)\s+black_end:([\d.]+)\s+black_duration:([\d.]+)`)
	reFreezeS = regexp.MustCompile(`freeze_start:\s*([\d.]+)`)
	reFreezeD = regexp.MustCompile(`freeze_duration:\s*([\d.]+)`)
	reCrop    = regexp.MustCompile(`crop=(\d+):(\d+):(\d+):(\d+)`)
	reMeta    = regexp.MustCompile(`lavfi\.signalstats\.(\w+)=([\-\d.]+)`)
)

// VideoDefect 는 화질 결함(블랙/정지/레터박스/휘도)을 검출한다.
func VideoDefect(fr *model.FileReport, probe *model.Probe, sp *spec.Spec, tl tools.Tools, deep bool) {
	if !deep {
		fr.Add(model.CheckResult{CheckID: "video.defect", Category: catVideo,
			Title: "화질 결함 검출", Severity: model.Skip, Message: "빠른 검사 모드: 결함 검출 생략(--deep)."})
		return
	}
	qv := spec.Map(sp.Quality(), "video")
	dur := probe.DurationSec()

	blackdetect(fr, spec.Map(qv, "blackdetect"), dur, tl)
	freezedetect(fr, spec.Map(qv, "freezedetect"), tl)
	cropdetect(fr, spec.Map(qv, "cropdetect"), probe, tl)
	signalstats(fr, spec.Map(qv, "signalstats"), tl)
}

func blackdetect(fr *model.FileReport, cfg map[string]any, dur float64, tl tools.Tools) {
	if !spec.Bool(cfg, "enabled", true) {
		return
	}
	minDur, _ := spec.Float(cfg, "black_min_duration", 2.0)
	picTh, _ := spec.Float(cfg, "picture_black_ratio", 0.98)
	pixTh, _ := spec.Float(cfg, "pixel_black_threshold", 0.10)
	vf := fmt.Sprintf("blackdetect=d=%g:pic_th=%g:pix_th=%g", minDur, picTh, pixTh)
	log, _ := tl.FilterStderr(fr.Path, vf, "")

	edge, _ := spec.Float(cfg, "max_edge_sec", 5.0)
	var segs []map[string]any
	total := 0.0
	for _, m := range reBlack.FindAllStringSubmatch(log, -1) {
		start := atof(m[1])
		end := atof(m[2])
		d := atof(m[3])
		if start <= edge || (dur > 0 && end >= dur-edge) {
			continue // 정상적인 시작/종료 여백 제외
		}
		segs = append(segs, map[string]any{"start": r2(start), "end": r2(end), "duration": r2(d)})
		total += d
	}
	ratio := 0.0
	if dur > 0 {
		ratio = total / dur
	}
	warnR, _ := spec.Float(cfg, "warn_total_ratio", 0.02)
	failR, _ := spec.Float(cfg, "fail_total_ratio", 0.05)
	var sev model.Severity
	var msg string
	switch {
	case ratio >= failR:
		sev = model.Fail
		msg = fmt.Sprintf("본편 내 블랙(무영상) 구간이 과다합니다: 누적 %.1fs (%.1f%%), %d개 구간. 신호 유실/변환 누락 의심.", total, ratio*100, len(segs))
	case ratio >= warnR || len(segs) > 0:
		sev = model.Warn
		msg = fmt.Sprintf("본편 내 블랙 구간 발견: 누적 %.1fs (%.1f%%), %d개 구간. 원본 대조 확인 권장.", total, ratio*100, len(segs))
	default:
		sev = model.Pass
		msg = "본편 내 이상 블랙 구간 없음."
	}
	fr.Add(model.CheckResult{CheckID: "video.blackdetect", Category: catVideo,
		Title: "블랙 프레임(무영상)", Severity: sev, Message: msg,
		Expected: fmt.Sprintf("블랙 비율 < %.0f%%", warnR*100),
		Actual:   fmt.Sprintf("%.1f%%", ratio*100),
		Evidence: map[string]any{"segments": clip(segs, 20), "total_black_sec": r2(total)}})
}

func freezedetect(fr *model.FileReport, cfg map[string]any, tl tools.Tools) {
	if !spec.Bool(cfg, "enabled", true) {
		return
	}
	noise, _ := spec.Float(cfg, "noise_db", -60)
	minDur, _ := spec.Float(cfg, "freeze_min_duration", 3.0)
	vf := fmt.Sprintf("freezedetect=n=%gdB:d=%g", noise, minDur)
	log, _ := tl.FilterStderr(fr.Path, vf, "")

	starts := reFreezeS.FindAllStringSubmatch(log, -1)
	durs := reFreezeD.FindAllStringSubmatch(log, -1)
	var segs []map[string]any
	for i, s := range starts {
		seg := map[string]any{"start": r2(atof(s[1]))}
		if i < len(durs) {
			seg["duration"] = r2(atof(durs[i][1]))
		}
		segs = append(segs, seg)
	}
	sev := model.Pass
	msg := "정지(프리즈) 구간 없음."
	if len(segs) > 0 {
		sev = model.Warn
		msg = fmt.Sprintf("정지(프리즈) 구간 %d개 발견. 캡처 멈춤/프레임 드롭 가능성 — 원본 대조 확인 권장.", len(segs))
	}
	fr.Add(model.CheckResult{CheckID: "video.freezedetect", Category: catVideo,
		Title: "정지 프레임(프리즈)", Severity: sev, Message: msg,
		Actual: fmt.Sprintf("%d개 구간", len(segs)), Evidence: map[string]any{"segments": clip(segs, 20)}})
}

func cropdetect(fr *model.FileReport, cfg map[string]any, probe *model.Probe, tl tools.Tools) {
	if !spec.Bool(cfg, "enabled", true) {
		return
	}
	v := probe.FirstStream("video")
	if v == nil || v.Width == 0 {
		return
	}
	width := v.Width
	log, _ := tl.FilterStderr(fr.Path, "cropdetect=limit=24:round=2", "", "-t", "60")
	crops := reCrop.FindAllStringSubmatch(log, -1)
	if len(crops) == 0 {
		fr.Add(model.CheckResult{CheckID: "video.cropdetect", Category: catVideo,
			Title: "레터/필러박스", Severity: model.Pass,
			Message: "유의미한 레터/필러박스 없음.", Actual: "crop 제안 없음"})
		return
	}
	last := crops[len(crops)-1]
	cw, _ := strconv.Atoi(last[1])
	ch, _ := strconv.Atoi(last[2])
	cx, _ := strconv.Atoi(last[3])
	cy, _ := strconv.Atoi(last[4])
	pillar := 0.0
	if width > 0 {
		pillar = float64(width-cw) / float64(width)
	}
	warnR, _ := spec.Float(cfg, "pillarbox_warn_ratio", 0.20)
	sev := model.Pass
	msg := fmt.Sprintf("종횡비/여백 정상 범위(유효폭 %d/%dpx).", cw, width)
	if pillar >= warnR {
		sev = model.Warn
		msg = fmt.Sprintf("좌우 검은띠(필러박스) 과다: 유효폭 %d/%dpx (%.0f%% 손실). 종횡비 변환 오류 가능성 확인 권장.", cw, width, pillar*100)
	}
	fr.Add(model.CheckResult{CheckID: "video.cropdetect", Category: catVideo,
		Title: "레터/필러박스(종횡비)", Severity: sev, Message: msg,
		Actual: fmt.Sprintf("crop=%d:%d:%d:%d", cw, ch, cx, cy),
		Evidence: map[string]any{"suggested_crop": fmt.Sprintf("%d:%d:%d:%d", cw, ch, cx, cy),
			"pillar_ratio": r3(pillar)}})
}

func signalstats(fr *model.FileReport, cfg map[string]any, tl tools.Tools) {
	if !spec.Bool(cfg, "enabled", true) {
		return
	}
	out, _ := tl.MetadataPrint(fr.Path, "fps=1,signalstats,metadata=print:file=-")
	var ymins, ymaxs, yavgs, satavgs []float64
	for _, m := range reMeta.FindAllStringSubmatch(out, -1) {
		val := atof(m[2])
		switch m[1] {
		case "YMIN":
			ymins = append(ymins, val)
		case "YMAX":
			ymaxs = append(ymaxs, val)
		case "YAVG":
			yavgs = append(yavgs, val)
		case "SATAVG":
			satavgs = append(satavgs, val)
		}
	}
	if len(yavgs) == 0 {
		fr.Add(model.CheckResult{CheckID: "video.signalstats", Category: catVideo,
			Title: "신호 레벨(휘도)", Severity: model.Skip, Message: "휘도 통계를 수집하지 못했습니다."})
		return
	}
	yAvg := mean(yavgs)
	satAvg := mean(satavgs)
	var issues []string
	if yAvg < 20 {
		issues = append(issues, fmt.Sprintf("전반적으로 매우 어두움(YAVG=%.0f)", yAvg))
	}
	if yAvg > 220 {
		issues = append(issues, fmt.Sprintf("전반적으로 매우 밝음(YAVG=%.0f)", yAvg))
	}
	if len(satavgs) > 0 && satAvg < 5 {
		issues = append(issues, fmt.Sprintf("채도 거의 없음(SATAVG=%.1f) — 흑백/색소실 가능", satAvg))
	}
	sev := model.Pass
	msg := fmt.Sprintf("휘도/채도 정상 범위(YAVG=%.0f).", yAvg)
	if len(issues) > 0 {
		sev = model.Warn
		msg = "휘도/채도 이상 신호: " + strings.Join(issues, "; ") + ". 원본 대조 확인 권장."
	}
	actual := map[string]any{"YAVG": r1(yAvg)}
	if len(ymins) > 0 {
		actual["YMIN"] = min(ymins)
	}
	if len(ymaxs) > 0 {
		actual["YMAX"] = max(ymaxs)
	}
	if len(satavgs) > 0 {
		actual["SATAVG"] = r1(satAvg)
	}
	fr.Add(model.CheckResult{CheckID: "video.signalstats", Category: catVideo,
		Title: "신호 레벨(휘도·채도)", Severity: sev, Message: msg, Actual: actual,
		Evidence: map[string]any{"samples": len(yavgs)}})
}

// ---- 수치 헬퍼 ----

func atof(s string) float64 { f, _ := strconv.ParseFloat(s, 64); return f }
func r1(v float64) float64  { return float64(int(v*10+0.5)) / 10 }
func r2(v float64) float64  { return float64(int(v*100+0.5)) / 100 }
func r3(v float64) float64  { return float64(int(v*1000+0.5)) / 1000 }

func mean(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := 0.0
	for _, x := range xs {
		s += x
	}
	return s / float64(len(xs))
}
func min(xs []float64) float64 {
	m := xs[0]
	for _, x := range xs {
		if x < m {
			m = x
		}
	}
	return m
}
func max(xs []float64) float64 {
	m := xs[0]
	for _, x := range xs {
		if x > m {
			m = x
		}
	}
	return m
}
func clip[T any](xs []T, n int) []T {
	if len(xs) > n {
		return xs[:n]
	}
	return xs
}
