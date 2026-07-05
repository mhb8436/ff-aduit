package checks

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"vqc/internal/model"
	"vqc/internal/spec"
)

const catMeta = "메타데이터 정합성"

// Metadata 는 메타데이터 파일(CSV)과 실제 납품파일을 대조한다(배치 단위).
// RFP 중점점검(메타데이터 구축-품질검증, 자료 검증-메타데이터 검증):
//   - 필수항목 누락, 날짜 형식 오류(오탈자성 입력)
//   - 시스템 메타데이터(해상도·코덱·컨테이너·재생시간)의 실측(ffprobe) 정합성
//
// metadataCSV 가 비어 있으면 inventory CSV 를 대신 사용한다(동일 파일에 메타 컬럼이
// 함께 있는 경우가 많다). 둘 다 없으면 SKIP.
func Metadata(metadataCSV, inventoryCSV string, files []*model.FileReport, sp *spec.Spec) []model.CheckResult {
	cfg := sp.Metadata()
	if !spec.Bool(cfg, "enabled", true) {
		return nil
	}

	path := metadataCSV
	source := "메타데이터"
	if path == "" {
		path = inventoryCSV // 납품목록에 메타 컬럼이 함께 있는 경우 재사용
		source = "납품목록(메타 컬럼)"
	}
	if path == "" {
		return []model.CheckResult{{CheckID: "metadata.crosscheck", Category: catMeta,
			Title: "메타데이터 대조", Severity: model.Skip,
			Message: "메타데이터 파일(--metadata) 미지정 — 메타데이터 정합성 대조를 생략합니다."}}
	}
	if _, err := os.Stat(path); err != nil {
		return []model.CheckResult{{CheckID: "metadata.crosscheck", Category: catMeta,
			Title: "메타데이터 대조", Severity: model.Error,
			Message: fmt.Sprintf("메타데이터 파일을 찾을 수 없습니다: %s", path)}}
	}

	rows, header, err := loadInventory(path, map[string]any{
		"filename_columns": toAnyList(spec.StrList(cfg, "filename_columns")),
	})
	if err != nil {
		return []model.CheckResult{{CheckID: "metadata.crosscheck", Category: catMeta,
			Title: "메타데이터 대조", Severity: model.Error,
			Message: fmt.Sprintf("메타데이터 로드 실패: %v", err)}}
	}

	// 전용 --metadata 없이 납품목록을 재사용했는데 메타데이터 컬럼이 전혀 없으면,
	// "메타데이터 미지정"으로 안내만 하고 상세 대조는 생략한다(오탐 방지).
	if metadataCSV == "" && !hasMetaColumns(header, cfg) {
		return []model.CheckResult{{CheckID: "metadata.crosscheck", Category: catMeta,
			Title: "메타데이터 대조", Severity: model.Skip,
			Message: "메타데이터 컬럼이 확인되지 않아 대조를 생략합니다(--metadata 로 메타데이터 CSV 지정)."}}
	}

	var out []model.CheckResult
	out = append(out, checkRequiredFields(rows, header, cfg, source))
	out = append(out, checkDateFields(rows, header, cfg))
	out = append(out, checkSystemMetadata(rows, files, cfg)...)
	return out
}

// checkRequiredFields 는 필수 메타데이터 항목의 누락 여부를 점검한다.
func checkRequiredFields(rows map[string]invRow, header []string, cfg map[string]any, source string) model.CheckResult {
	required := spec.StrList(cfg, "required_columns")
	if len(required) == 0 {
		return model.CheckResult{CheckID: "metadata.required", Category: catMeta,
			Title: "필수항목 입력", Severity: model.Skip,
			Message: "필수 메타데이터 항목(metadata.required_columns)이 정의되지 않아 생략."}
	}
	// 실제 존재하는 컬럼명으로 해석(대소문자 무시)
	cols := map[string]string{}
	for _, want := range required {
		if c := pickColumnName(header, []string{want}); c != "" {
			cols[want] = c
		}
	}
	var missingCols []string
	for _, want := range required {
		if _, ok := cols[want]; !ok {
			missingCols = append(missingCols, want)
		}
	}
	var emptyCells []map[string]any
	for name, row := range rows {
		var blanks []string
		for _, want := range required {
			col, ok := cols[want]
			if !ok {
				continue
			}
			if strings.TrimSpace(row[col]) == "" {
				blanks = append(blanks, want)
			}
		}
		if len(blanks) > 0 {
			emptyCells = append(emptyCells, map[string]any{"file": name, "missing": blanks})
		}
	}
	sort.Strings(missingCols)

	sev := model.Pass
	msg := fmt.Sprintf("%s 필수항목 전건 입력 완료.", source)
	switch {
	case len(missingCols) > 0:
		sev = model.Fail
		msg = fmt.Sprintf("필수 메타데이터 컬럼 자체가 없음: %v. 표준항목 미구축.", missingCols)
	case len(emptyCells) > 0:
		sev = model.Fail
		msg = fmt.Sprintf("필수 메타데이터 값 누락 %d개 항목. 표준항목 입력기준 미준수 — 보완 필요.", len(emptyCells))
	}
	return model.CheckResult{CheckID: "metadata.required", Category: catMeta,
		Title: "필수항목 입력", Severity: sev, Message: msg,
		Expected: required,
		Actual:   fmt.Sprintf("컬럼누락 %d, 값누락 %d건", len(missingCols), len(emptyCells)),
		Evidence: map[string]any{"missing_columns": missingCols, "empty_values": clip(emptyCells, 30)}}
}

// checkDateFields 는 날짜 컬럼의 형식·유효성을 점검한다(오탈자성 오류 검출).
func checkDateFields(rows map[string]invRow, header []string, cfg map[string]any) model.CheckResult {
	dateCols := spec.StrList(cfg, "date_columns")
	if len(dateCols) == 0 {
		return model.CheckResult{CheckID: "metadata.date", Category: catMeta,
			Title: "날짜 형식", Severity: model.Skip,
			Message: "날짜 컬럼(metadata.date_columns)이 정의되지 않아 생략."}
	}
	layouts := dateLayouts(spec.StrList(cfg, "date_formats"))
	if len(layouts) == 0 {
		layouts = []string{"20060102", "2006-01-02", "2006.01.02", "2006/01/02"}
	}
	var bad []map[string]any
	for name, row := range rows {
		for _, dc := range dateCols {
			col := pickColumnName(header, []string{dc})
			if col == "" {
				continue
			}
			val := strings.TrimSpace(row[col])
			if val == "" {
				continue // 누락은 필수항목 점검에서 다룸
			}
			if !parseAnyDate(val, layouts) {
				bad = append(bad, map[string]any{"file": name, "column": dc, "value": val})
			}
		}
	}
	sev := model.Pass
	msg := "날짜 항목이 모두 유효한 형식."
	if len(bad) > 0 {
		sev = model.Warn
		msg = fmt.Sprintf("날짜 형식/값 오류 %d건. 입력 오탈자 또는 형식 불일치 — 보정 필요.", len(bad))
	}
	return model.CheckResult{CheckID: "metadata.date", Category: catMeta,
		Title: "날짜 형식", Severity: sev, Message: msg,
		Actual: fmt.Sprintf("오류 %d건", len(bad)), Evidence: map[string]any{"invalid": clip(bad, 30)}}
}

// checkSystemMetadata 는 목록에 기재된 시스템 메타데이터(해상도/코덱/컨테이너/재생시간)를
// 실측(ffprobe)값과 대조한다. RFP: "자동추출 항목 정합성 확인".
func checkSystemMetadata(rows map[string]invRow, files []*model.FileReport, cfg map[string]any) []model.CheckResult {
	sysCfg := spec.Map(cfg, "system_check")
	if len(sysCfg) == 0 {
		return []model.CheckResult{{CheckID: "metadata.system", Category: catMeta,
			Title: "시스템 메타데이터 정합성", Severity: model.Skip,
			Message: "시스템 메타데이터 대조 설정(metadata.system_check)이 없어 생략."}}
	}
	resCol := spec.Str(sysCfg, "resolution_column")
	codecCol := spec.Str(sysCfg, "codec_column")
	contCol := spec.Str(sysCfg, "container_column")
	durCol := spec.Str(sysCfg, "duration_column")
	durTol, _ := spec.Float(sysCfg, "duration_tolerance_sec", 2.0)

	var mism []map[string]any
	compared := 0
	for _, f := range files {
		if f.Probe == nil {
			continue
		}
		name := strings.ToLower(filepath.Base(f.Path))
		row, ok := rows[name]
		if !ok {
			continue
		}
		compared++
		v := f.Probe.FirstStream("video")

		if resCol != "" && v != nil {
			if listed := strings.TrimSpace(row[resCol]); listed != "" {
				actual := fmt.Sprintf("%dx%d", v.Width, v.Height)
				if !sameResolution(listed, actual) {
					mism = append(mism, mrow(name, "해상도", listed, actual))
				}
			}
		}
		if codecCol != "" && v != nil {
			if listed := strings.TrimSpace(row[codecCol]); listed != "" {
				if !strings.EqualFold(normCodec(listed), normCodec(v.CodecName)) {
					mism = append(mism, mrow(name, "코덱", listed, v.CodecName))
				}
			}
		}
		if contCol != "" {
			if listed := strings.TrimSpace(row[contCol]); listed != "" {
				if !containerMatch(listed, f.Probe.Format.FormatName) {
					mism = append(mism, mrow(name, "컨테이너", listed, f.Probe.Format.FormatName))
				}
			}
		}
		if durCol != "" {
			if listed := parseDuration(row[durCol]); listed >= 0 {
				actual := f.Probe.DurationSec()
				if actual > 0 && absf(listed-actual) > durTol {
					mism = append(mism, mrow(name, "재생시간",
						fmt.Sprintf("%.1fs", listed), fmt.Sprintf("%.1fs", actual)))
				}
			}
		}
	}

	sev := model.Pass
	msg := fmt.Sprintf("시스템 메타데이터 %d개 파일 전건 실측 일치.", compared)
	if compared == 0 {
		sev = model.Skip
		msg = "실측과 대조할 매칭 파일이 없어 시스템 메타데이터 대조를 생략."
	} else if len(mism) > 0 {
		sev = model.Fail
		msg = fmt.Sprintf("메타데이터-실측 불일치 %d건. 자동추출 메타데이터가 실제 파일과 다릅니다 — 메타 재추출/보정 필요.", len(mism))
	}
	return []model.CheckResult{{CheckID: "metadata.system", Category: catMeta,
		Title: "시스템 메타데이터 정합성(목록 vs 실측)", Severity: sev, Message: msg,
		Actual: fmt.Sprintf("불일치 %d건", len(mism)), Evidence: map[string]any{"mismatches": clip(mism, 40)}}}
}

// ---- 헬퍼 ----

// hasMetaColumns 는 헤더에 필수/날짜/시스템 메타 컬럼이 하나라도 있는지 본다.
func hasMetaColumns(header []string, cfg map[string]any) bool {
	cands := append([]string{}, spec.StrList(cfg, "required_columns")...)
	cands = append(cands, spec.StrList(cfg, "date_columns")...)
	sysCfg := spec.Map(cfg, "system_check")
	for _, k := range []string{"resolution_column", "codec_column", "container_column"} {
		if c := spec.Str(sysCfg, k); c != "" {
			cands = append(cands, c)
		}
	}
	for _, c := range cands {
		if pickColumnName(header, []string{c}) != "" {
			return true
		}
	}
	return false
}

func mrow(file, field, listed, actual string) map[string]any {
	return map[string]any{"file": file, "field": field, "listed": listed, "actual": actual}
}

func toAnyList(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}

// sameResolution 은 "1920x1080" / "1920X1080" / "1920*1080" 등을 정규화 비교.
func sameResolution(a, b string) bool {
	return normRes(a) == normRes(b)
}

func normRes(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.NewReplacer("×", "x", "*", "x", " ", "").Replace(s)
	return s
}

func normCodec(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	// 표기 관용 정규화: h.264/avc → h264, mpeg-2 → mpeg2video 등
	switch s {
	case "h.264", "avc", "avc1", "x264":
		return "h264"
	case "h.265", "hevc":
		return "hevc"
	case "mpeg-2", "mpeg2", "mpeg2video":
		return "mpeg2video"
	}
	return strings.ReplaceAll(s, ".", "")
}

// containerMatch 는 목록 표기(mp4, mxf, quicktime 등)가 ffprobe format_name 에 포함되는지 본다.
func containerMatch(listed, formatName string) bool {
	l := strings.ToLower(strings.TrimSpace(listed))
	fn := strings.ToLower(formatName)
	if l == "quicktime" {
		l = "mov"
	}
	for _, part := range strings.Split(fn, ",") {
		if strings.TrimSpace(part) == l {
			return true
		}
	}
	return strings.Contains(fn, l)
}

// dateLayouts 는 사용자 친화 토큰(YYYYMMDD 등)을 Go time 레이아웃으로 변환한다.
func dateLayouts(tokens []string) []string {
	rep := strings.NewReplacer(
		"YYYY", "2006", "MM", "01", "DD", "02",
		"HH", "15", "mm", "04", "SS", "05")
	out := make([]string, 0, len(tokens))
	for _, t := range tokens {
		out = append(out, rep.Replace(t))
	}
	return out
}

func parseAnyDate(val string, layouts []string) bool {
	for _, l := range layouts {
		if _, err := time.Parse(l, val); err == nil {
			return true
		}
	}
	return false
}
