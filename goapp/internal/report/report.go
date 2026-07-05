// Package report 는 감리 증적용 리포트(콘솔/HTML/CSV/JSON)를 생성한다.
package report

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"vqc/internal/model"
)

// WriteJSON 은 배치 결과를 JSON 파일로 저장.
func WriteJSON(b *model.BatchReport, path string) error {
	s, err := b.ToJSON()
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte(s), 0o644)
}

// WriteCSV 는 항목 단위 상세 결과를 CSV(감리조서 첨부용)로 저장.
func WriteCSV(b *model.BatchReport, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	f.WriteString("\ufeff") // 엑셀 한글 호환 BOM
	w := csv.NewWriter(f)
	defer w.Flush()
	w.Write([]string{"파일", "프로파일", "종합판정", "분류", "점검항목", "판정", "기대값", "실측값", "판정사유"})
	for _, fr := range b.Files {
		base := filepath.Base(fr.Path)
		verdict := fr.Verdict().String()
		for _, r := range fr.Results {
			w.Write([]string{base, fr.Profile, verdict, r.Category, r.Title,
				r.Severity.String(), short(r.Expected), short(r.Actual), r.Message})
		}
	}
	for _, r := range b.Inventory {
		w.Write([]string{"(배치)", "-", "-", r.Category, r.Title,
			r.Severity.String(), short(r.Expected), short(r.Actual), r.Message})
	}
	return nil
}

// PrintConsoleSummary 는 터미널 요약을 출력한다.
func PrintConsoleSummary(b *model.BatchReport) {
	s := b.Summary()
	bar := strings.Repeat("=", 64)
	proj, _ := b.SpecMeta["project"].(string)
	fmt.Println("\n" + bar)
	fmt.Printf(" 영상 검사 결과 요약 — %s\n", proj)
	fmt.Println(bar)
	smp := b.Sampling
	fmt.Printf("  모집단 %v개 중 표본 %v개 검사 (방식: %v)\n",
		smp["population"], firstNonNil(smp["selected_count"], smp["size"]), smp["method"])
	fmt.Printf("  검사 파일: %v개   합격률: %v%%\n", s["total_files"], s["pass_rate"])
	bv, _ := s["by_verdict"].(map[string]int)
	fmt.Printf("  판정 → PASS %d / WARN %d / FAIL %d / ERROR %d\n",
		bv["PASS"], bv["WARN"], bv["FAIL"], bv["ERROR"])

	var flagged []*model.FileReport
	for _, fr := range b.Files {
		if fr.Verdict().Rank() >= model.Warn.Rank() {
			flagged = append(flagged, fr)
		}
	}
	if len(flagged) > 0 {
		fmt.Println(strings.Repeat("-", 64))
		fmt.Println("  [지적/주의 파일]")
		for _, fr := range flagged {
			fmt.Printf("   %-5s %s\n", fr.Verdict().String(), filepath.Base(fr.Path))
			for _, r := range fr.Failures() {
				fmt.Printf("          - [%s] %s: %s\n", r.Severity.String(), r.Title, r.Message)
			}
		}
	}
	var invIssues []model.CheckResult
	for _, r := range b.Inventory {
		if r.Severity.Rank() >= model.Warn.Rank() {
			invIssues = append(invIssues, r)
		}
	}
	if len(invIssues) > 0 {
		fmt.Println(strings.Repeat("-", 64))
		fmt.Println("  [납품목록 정합성 지적]")
		for _, r := range invIssues {
			fmt.Printf("   [%s] %s: %s\n", r.Severity.String(), r.Title, r.Message)
		}
	}
	fmt.Println(bar + "\n")
}

func short(v any) string {
	if v == nil {
		return ""
	}
	s := fmt.Sprintf("%v", v)
	if len(s) > 120 {
		return s[:117] + "..."
	}
	return s
}

func firstNonNil(a, b any) any {
	if a != nil {
		return a
	}
	return b
}
