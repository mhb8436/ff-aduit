package report

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/xuri/excelize/v2"

	"vqc/internal/model"
)

// 요약 시트에 표시할 카테고리(순서 고정).
var summaryCats = []struct{ key, label string }{
	{"무결성/재생가능", "무결성"},
	{"기술규격", "기술규격"},
	{"화질", "화질"},
	{"음질", "음질"},
	{"동기화(싱크)", "싱크"},
	{"파일명/구조", "파일명"},
}

// 판정별 셀 색상(배경/글자).
var verdictFill = map[string][2]string{
	"PASS":  {"E6F4EA", "1A7F37"},
	"WARN":  {"FFF8C5", "9A6700"},
	"FAIL":  {"FFEBE9", "CF222E"},
	"ERROR": {"FFCECB", "82071E"},
	"SKIP":  {"EFF2F4", "57606A"},
	"":      {"FFFFFF", "1F2328"},
}

// WriteXLSX 는 감리용 종합 엑셀(.xlsx)을 생성한다.
//
//	요약   : 영상 파일 1행씩 — 카테고리별 판정을 색으로 구분, 주요 지적 요약
//	상세   : 점검 항목 1행씩 — 기대값·실측값·판정사유
//	납품목록: 배치 단위 정합성 점검
//	표본정보: 표본 선정 파라미터
func WriteXLSX(b *model.BatchReport, path string) error {
	f := excelize.NewFile()
	defer f.Close()

	styles := newStyleSet(f)
	writeSummarySheet(f, b, styles)
	writeDetailSheet(f, b, styles)
	writeInventorySheet(f, b, styles)
	writeSamplingSheet(f, b, styles)

	// 기본 시트 제거, 첫 시트를 활성화
	f.DeleteSheet("Sheet1")
	if idx, err := f.GetSheetIndex("요약"); err == nil {
		f.SetActiveSheet(idx)
	}
	return f.SaveAs(path)
}

type styleSet struct {
	header  int
	verdict map[string]int
	wrap    int
	mono    int
}

func newStyleSet(f *excelize.File) *styleSet {
	s := &styleSet{verdict: map[string]int{}}
	s.header, _ = f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "1F2328", Size: 10},
		Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"EDF1F3"}},
		Alignment: &excelize.Alignment{Vertical: "center", WrapText: true},
		Border:    box("D0D7DE"),
	})
	for v, c := range verdictFill {
		s.verdict[v], _ = f.NewStyle(&excelize.Style{
			Font:      &excelize.Font{Bold: true, Color: c[1], Size: 10},
			Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{c[0]}},
			Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
			Border:    box("E6EAED"),
		})
	}
	s.wrap, _ = f.NewStyle(&excelize.Style{
		Alignment: &excelize.Alignment{Vertical: "top", WrapText: true},
		Border:    box("EAEEF2"),
	})
	s.mono, _ = f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Family: "Consolas", Size: 9, Color: "57606A"},
		Alignment: &excelize.Alignment{Vertical: "top", WrapText: true},
		Border:    box("EAEEF2"),
	})
	return s
}

func box(color string) []excelize.Border {
	return []excelize.Border{
		{Type: "left", Color: color, Style: 1},
		{Type: "right", Color: color, Style: 1},
		{Type: "top", Color: color, Style: 1},
		{Type: "bottom", Color: color, Style: 1},
	}
}

// ---- 요약 시트 ----

func writeSummarySheet(f *excelize.File, b *model.BatchReport, st *styleSet) {
	sheet := "요약"
	f.NewSheet(sheet)

	headers := []string{"파일명", "프로파일", "종합판정"}
	for _, c := range summaryCats {
		headers = append(headers, c.label)
	}
	headers = append(headers, "재생시간(초)", "영상", "오디오", "주요 지적")

	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet, cell, h)
		f.SetCellStyle(sheet, cell, cell, st.header)
	}

	row := 2
	for _, fr := range b.Files {
		catV := categoryVerdicts(fr)
		col := 1
		put := func(v any, style int) {
			cell, _ := excelize.CoordinatesToCellName(col, row)
			f.SetCellValue(sheet, cell, v)
			if style >= 0 {
				f.SetCellStyle(sheet, cell, cell, style)
			}
			col++
		}
		put(filepath.Base(fr.Path), st.wrap)
		put(profileLabel(fr.Profile), st.wrap)
		put(fr.Verdict().String(), st.verdict[fr.Verdict().String()])
		for _, c := range summaryCats {
			v := catV[c.key]
			put(v, st.verdict[v])
		}
		put(durationStr(fr), st.wrap)
		put(videoStr(fr), st.mono)
		put(audioStr(fr), st.mono)
		put(defectSummary(fr), st.wrap)
		row++
	}

	// 열 너비
	widths := map[string]float64{"A": 34, "B": 12, "C": 10}
	for col, w := range widths {
		f.SetColWidth(sheet, col, col, w)
	}
	// 카테고리 열(D~H) 균일
	f.SetColWidth(sheet, "D", colLetter(3+len(summaryCats)), 9)
	base := 3 + len(summaryCats)
	f.SetColWidth(sheet, colLetter(base+1), colLetter(base+1), 12) // 재생시간
	f.SetColWidth(sheet, colLetter(base+2), colLetter(base+3), 22) // 영상/오디오
	f.SetColWidth(sheet, colLetter(base+4), colLetter(base+4), 60) // 주요 지적

	// 머리글 고정
	f.SetPanes(sheet, &excelize.Panes{
		Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft",
	})
	f.SetRowHeight(sheet, 1, 22)
}

// ---- 상세 시트 ----

func writeDetailSheet(f *excelize.File, b *model.BatchReport, st *styleSet) {
	sheet := "상세"
	f.NewSheet(sheet)
	headers := []string{"파일", "프로파일", "종합판정", "분류", "점검항목", "판정", "기대값", "실측값", "판정사유"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet, cell, h)
		f.SetCellStyle(sheet, cell, cell, st.header)
	}
	row := 2
	for _, fr := range b.Files {
		base := filepath.Base(fr.Path)
		verdict := fr.Verdict().String()
		for _, r := range fr.Results {
			vals := []any{base, profileLabel(fr.Profile), verdict, r.Category, r.Title,
				r.Severity.String(), short(r.Expected), short(r.Actual), r.Message}
			for i, v := range vals {
				cell, _ := excelize.CoordinatesToCellName(i+1, row)
				f.SetCellValue(sheet, cell, v)
				switch i {
				case 5:
					f.SetCellStyle(sheet, cell, cell, st.verdict[r.Severity.String()])
				case 6, 7:
					f.SetCellStyle(sheet, cell, cell, st.mono)
				default:
					f.SetCellStyle(sheet, cell, cell, st.wrap)
				}
			}
			row++
		}
	}
	for _, cw := range []struct {
		c string
		w float64
	}{{"A", 30}, {"B", 12}, {"C", 10}, {"D", 14}, {"E", 20}, {"F", 8}, {"G", 22}, {"H", 22}, {"I", 60}} {
		f.SetColWidth(sheet, cw.c, cw.c, cw.w)
	}
	f.SetPanes(sheet, &excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"})
}

// ---- 납품목록 시트 ----

func writeInventorySheet(f *excelize.File, b *model.BatchReport, st *styleSet) {
	sheet := "납품목록"
	f.NewSheet(sheet)
	headers := []string{"점검항목", "판정", "판정사유"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet, cell, h)
		f.SetCellStyle(sheet, cell, cell, st.header)
	}
	row := 2
	for _, r := range b.Inventory {
		c1, _ := excelize.CoordinatesToCellName(1, row)
		c2, _ := excelize.CoordinatesToCellName(2, row)
		c3, _ := excelize.CoordinatesToCellName(3, row)
		f.SetCellValue(sheet, c1, r.Title)
		f.SetCellStyle(sheet, c1, c1, st.wrap)
		f.SetCellValue(sheet, c2, r.Severity.String())
		f.SetCellStyle(sheet, c2, c2, st.verdict[r.Severity.String()])
		f.SetCellValue(sheet, c3, r.Message)
		f.SetCellStyle(sheet, c3, c3, st.wrap)
		row++
	}
	f.SetColWidth(sheet, "A", "A", 28)
	f.SetColWidth(sheet, "B", "B", 8)
	f.SetColWidth(sheet, "C", "C", 80)
	f.SetPanes(sheet, &excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"})
}

// ---- 표본정보 시트 ----

func writeSamplingSheet(f *excelize.File, b *model.BatchReport, st *styleSet) {
	sheet := "표본정보"
	f.NewSheet(sheet)
	f.SetCellValue(sheet, "A1", "항목")
	f.SetCellValue(sheet, "B1", "값")
	f.SetCellStyle(sheet, "A1", "B1", st.header)

	rows := [][2]string{
		{"사업", str(b.SpecMeta["project"])},
		{"발주기관", str(b.SpecMeta["authority"])},
		{"검사기준", str(b.SpecMeta["spec_version"])},
		{"검사일시(시작)", b.StartedAt},
		{"검사일시(종료)", b.FinishedAt},
		{"표본 방식", str(b.Sampling["method"])},
		{"모집단", str(b.Sampling["population"])},
		{"표본 수", str(firstNonNil(b.Sampling["selected_count"], b.Sampling["size"]))},
	}
	if params, ok := b.Sampling["params"].(map[string]any); ok {
		keys := make([]string, 0, len(params))
		for k := range params {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			rows = append(rows, [2]string{"파라미터 · " + k, str(params[k])})
		}
	}
	for i, kv := range rows {
		r := i + 2
		a, _ := excelize.CoordinatesToCellName(1, r)
		bcell, _ := excelize.CoordinatesToCellName(2, r)
		f.SetCellValue(sheet, a, kv[0])
		f.SetCellStyle(sheet, a, a, st.wrap)
		f.SetCellValue(sheet, bcell, kv[1])
		f.SetCellStyle(sheet, bcell, bcell, st.wrap)
	}
	f.SetColWidth(sheet, "A", "A", 24)
	f.SetColWidth(sheet, "B", "B", 60)
}

// ---- 헬퍼 ----

// categoryVerdicts 는 파일의 카테고리별 최악 판정을 계산한다.
func categoryVerdicts(fr *model.FileReport) map[string]string {
	worst := map[string]model.Severity{}
	seen := map[string]bool{}
	for _, r := range fr.Results {
		if !seen[r.Category] || r.Severity.Rank() > worst[r.Category].Rank() {
			worst[r.Category] = r.Severity
			seen[r.Category] = true
		}
	}
	out := map[string]string{}
	for _, c := range summaryCats {
		if seen[c.key] {
			out[c.key] = worst[c.key].String()
		} else {
			out[c.key] = ""
		}
	}
	return out
}

// defectSummary 는 WARN/FAIL/ERROR 항목의 제목을 모아 한 줄로.
func defectSummary(fr *model.FileReport) string {
	var parts []string
	for _, r := range fr.Failures() {
		parts = append(parts, fmt.Sprintf("[%s] %s", r.Severity.String(), r.Title))
	}
	if len(parts) == 0 {
		return "이상 없음"
	}
	return strings.Join(parts, "; ")
}

func profileLabel(p string) string {
	switch p {
	case "preservation_master":
		return "보존마스터"
	case "access_copy":
		return "서비스본"
	}
	return p
}

func durationStr(fr *model.FileReport) any {
	if fr.Probe == nil {
		return ""
	}
	d := fr.Probe.DurationSec()
	if d <= 0 {
		return ""
	}
	return int(d + 0.5)
}

func videoStr(fr *model.FileReport) string {
	if fr.Probe == nil {
		return ""
	}
	v := fr.Probe.FirstStream("video")
	if v == nil {
		return "(없음)"
	}
	fr2 := v.AvgFrameRate
	if fr2 == "" || fr2 == "0/0" {
		fr2 = v.RFrameRate
	}
	return fmt.Sprintf("%s %dx%d %gfps", v.CodecName, v.Width, v.Height, model.ParseRate(fr2))
}

func audioStr(fr *model.FileReport) string {
	if fr.Probe == nil {
		return ""
	}
	a := fr.Probe.FirstStream("audio")
	if a == nil {
		return "(없음)"
	}
	return fmt.Sprintf("%s %dch %sHz", a.CodecName, a.Channels, a.SampleRate)
}

func str(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%v", v)
}

func colLetter(n int) string {
	name, _ := excelize.ColumnNumberToName(n)
	return name
}
