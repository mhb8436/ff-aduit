package checks

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"vqc/internal/model"
	"vqc/internal/spec"
)

const catInv = "납품목록 정합성"

// Inventory 는 납품목록(CSV)과 실제 파일 집합을 대조한다(배치 단위).
// RFP 중점점검: "중복·누락 여부", "매핑 정합성", "납품자료 목록의 정합성".
func Inventory(inventoryCSV string, files []*model.FileReport, sp *spec.Spec, baseDir string) []model.CheckResult {
	var out []model.CheckResult
	invCfg := sp.Inventory()

	actualLower := make([]string, 0, len(files))
	for _, f := range files {
		actualLower = append(actualLower, strings.ToLower(filepath.Base(f.Path)))
	}

	// 1) 중복 파일명
	seen := map[string]bool{}
	dupSet := map[string]bool{}
	for _, n := range actualLower {
		if seen[n] {
			dupSet[n] = true
		}
		seen[n] = true
	}
	dups := keys(dupSet)
	sort.Strings(dups)
	{
		sev := model.Pass
		msg := "중복 파일명 없음."
		if len(dups) > 0 {
			sev = model.Warn
			msg = fmt.Sprintf("중복 파일명 %d건: %v. 관리·매핑 오류 우려.", len(dups), clip(dups, 10))
		}
		out = append(out, model.CheckResult{CheckID: "inventory.duplicates", Category: catInv,
			Title: "파일명 중복", Severity: sev, Message: msg, Actual: clip(dups, 20)})
	}

	// 2) 납품목록 대조
	if inventoryCSV == "" {
		out = append(out, model.CheckResult{CheckID: "inventory.crosscheck", Category: catInv,
			Title: "납품목록 대조", Severity: model.Skip,
			Message: "납품목록(CSV) 미지정 — 목록-실물 대조 생략(--inventory 로 지정)."})
		return out
	}
	if _, err := os.Stat(inventoryCSV); err != nil {
		out = append(out, model.CheckResult{CheckID: "inventory.crosscheck", Category: catInv,
			Title: "납품목록 대조", Severity: model.Error,
			Message: fmt.Sprintf("납품목록 파일을 찾을 수 없습니다: %s", inventoryCSV)})
		return out
	}

	rows, header, err := loadInventory(inventoryCSV, invCfg)
	if err != nil {
		out = append(out, model.CheckResult{CheckID: "inventory.crosscheck", Category: catInv,
			Title: "납품목록 대조", Severity: model.Error, Message: err.Error()})
		return out
	}

	invNames := map[string]bool{}
	for k := range rows {
		invNames[k] = true
	}
	actSet := map[string]bool{}
	for _, n := range actualLower {
		actSet[n] = true
	}

	missing := diff(invNames, actSet)   // 목록엔 있으나 실물 없음
	unlisted := diff(actSet, invNames)  // 실물 있으나 목록에 없음
	sort.Strings(missing)
	sort.Strings(unlisted)

	{
		sev := model.Pass
		msg := "목록의 모든 항목이 실물로 존재."
		if len(missing) > 0 {
			sev = model.Fail
			msg = fmt.Sprintf("목록에 있으나 실물이 없는 파일 %d건: %v. 납품 누락.", len(missing), clip(missing, 10))
		}
		out = append(out, model.CheckResult{CheckID: "inventory.missing", Category: catInv,
			Title: "목록 대비 누락(실물 없음)", Severity: sev, Message: msg,
			Expected: fmt.Sprintf("목록 %d건", len(invNames)),
			Actual:   fmt.Sprintf("누락 %d건", len(missing)),
			Evidence: map[string]any{"missing": clip(missing, 50)}})
	}
	{
		sev := model.Pass
		msg := "모든 실물이 목록에 등재됨."
		if len(unlisted) > 0 {
			sev = model.Warn
			msg = fmt.Sprintf("목록에 없는 실물 파일 %d건: %v. 목록 갱신 필요.", len(unlisted), clip(unlisted, 10))
		}
		out = append(out, model.CheckResult{CheckID: "inventory.unlisted", Category: catInv,
			Title: "목록 미등재(실물 있음)", Severity: sev, Message: msg,
			Actual:   fmt.Sprintf("미등재 %d건", len(unlisted)),
			Evidence: map[string]any{"unlisted": clip(unlisted, 50)}})
	}

	// 3) 재생시간 정합성
	out = append(out, checkDuration(rows, header, files, invCfg))
	return out
}

type invRow map[string]string

func loadInventory(path string, cfg map[string]any) (map[string]invRow, []string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	records, err := r.ReadAll()
	if err != nil {
		return nil, nil, err
	}
	if len(records) == 0 {
		return map[string]invRow{}, nil, nil
	}
	header := records[0]
	// BOM 제거
	if len(header) > 0 {
		header[0] = strings.TrimPrefix(header[0], "\ufeff")
	}
	fnCol := pickColumn(header, spec.StrList(cfg, "filename_columns"))
	if fnCol < 0 {
		return nil, nil, fmt.Errorf("납품목록에서 파일명 컬럼을 찾지 못했습니다 (후보: %v)", spec.StrList(cfg, "filename_columns"))
	}
	rows := map[string]invRow{}
	for _, rec := range records[1:] {
		if fnCol >= len(rec) {
			continue
		}
		name := strings.TrimSpace(rec[fnCol])
		if name == "" {
			continue
		}
		row := invRow{}
		for i, h := range header {
			if i < len(rec) {
				row[h] = rec[i]
			}
		}
		rows[strings.ToLower(filepath.Base(name))] = row
	}
	return rows, header, nil
}

func checkDuration(rows map[string]invRow, header []string, files []*model.FileReport, cfg map[string]any) model.CheckResult {
	tol, _ := spec.Float(cfg, "duration_tolerance_sec", 2.0)
	durCol := pickColumnName(header, spec.StrList(cfg, "duration_columns"))
	if durCol == "" {
		return model.CheckResult{CheckID: "inventory.duration", Category: catInv,
			Title: "재생시간 정합성", Severity: model.Skip,
			Message: "납품목록에 재생시간 컬럼이 없어 대조 생략."}
	}
	var mism []map[string]any
	for _, f := range files {
		name := strings.ToLower(filepath.Base(f.Path))
		row, ok := rows[name]
		if !ok {
			continue
		}
		listed := parseDuration(row[durCol])
		if listed < 0 || f.Probe == nil {
			continue
		}
		actual := f.Probe.DurationSec()
		if actual == 0 {
			continue
		}
		if absf(listed-actual) > tol {
			mism = append(mism, map[string]any{
				"file": filepath.Base(f.Path), "listed": r1(listed), "actual": r1(actual)})
		}
	}
	sev := model.Pass
	msg := "목록 재생시간과 실측이 허용오차 내 일치."
	if len(mism) > 0 {
		sev = model.Warn
		msg = fmt.Sprintf("재생시간 불일치 %d건(허용오차 %gs 초과). 변환 누락/절단 또는 목록 오기 확인 필요.", len(mism), tol)
	}
	return model.CheckResult{CheckID: "inventory.duration", Category: catInv,
		Title: "재생시간 정합성(목록 vs 실측)", Severity: sev, Message: msg,
		Actual: fmt.Sprintf("불일치 %d건", len(mism)), Evidence: map[string]any{"mismatches": clip(mism, 30)}}
}

// ---- 헬퍼 ----

func pickColumn(header, candidates []string) int {
	low := map[string]int{}
	for i, h := range header {
		low[strings.ToLower(strings.TrimSpace(h))] = i
	}
	for _, c := range candidates {
		if i, ok := low[strings.ToLower(c)]; ok {
			return i
		}
	}
	return -1
}

func pickColumnName(header, candidates []string) string {
	i := pickColumn(header, candidates)
	if i < 0 {
		return ""
	}
	return header[i]
}

// parseDuration 은 "01:23:45" 또는 초 숫자를 초로 변환(실패 시 -1).
func parseDuration(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return -1
	}
	if strings.Contains(s, ":") {
		parts := strings.Split(s, ":")
		vals := make([]float64, len(parts))
		for i, p := range parts {
			vals[i] = atof(strings.TrimSpace(p))
		}
		for len(vals) < 3 {
			vals = append([]float64{0}, vals...)
		}
		n := len(vals)
		return vals[n-3]*3600 + vals[n-2]*60 + vals[n-1]
	}
	if v := atof(s); v > 0 || s == "0" {
		return v
	}
	return -1
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func diff(a, b map[string]bool) []string {
	var out []string
	for k := range a {
		if !b[k] {
			out = append(out, k)
		}
	}
	return out
}
