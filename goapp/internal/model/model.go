// Package model 은 검사 결과 데이터 구조와 판정 체계를 정의한다.
//
// 판정 체계
//
//	PASS  : 기준 충족
//	WARN  : 경계/주의 — 감리의견(개선권고) 대상
//	FAIL  : 기준 미달 — 시정조치 대상
//	ERROR : 검사 자체 실패(파일 손상 등) — 재생 불가로 간주
//	SKIP  : 해당 항목 미적용
package model

import "encoding/json"

// Severity 는 판정 등급. 값이 클수록 심각하다(정렬·종합판정에 사용).
type Severity int

const (
	Skip Severity = iota
	Pass
	Warn
	Fail
	Error
)

func (s Severity) String() string {
	switch s {
	case Skip:
		return "SKIP"
	case Pass:
		return "PASS"
	case Warn:
		return "WARN"
	case Fail:
		return "FAIL"
	case Error:
		return "ERROR"
	}
	return "UNKNOWN"
}

// Rank 는 종합판정(가장 심각한 항목) 산정용 순위.
func (s Severity) Rank() int { return int(s) }

// MarshalJSON 은 문자열("PASS" 등)로 직렬화한다.
func (s Severity) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.String())
}

// CheckResult 는 개별 점검 항목 1건의 결과.
type CheckResult struct {
	CheckID  string         `json:"check_id"`
	Category string         `json:"category"`
	Title    string         `json:"title"`
	Severity Severity       `json:"severity"`
	Message  string         `json:"message"`
	Expected any            `json:"expected,omitempty"`
	Actual   any            `json:"actual,omitempty"`
	Evidence map[string]any `json:"evidence,omitempty"`
}

// FileReport 는 파일 1개에 대한 검사 결과 묶음.
type FileReport struct {
	Path    string        `json:"path"`
	Profile string        `json:"profile"`
	Probe   *Probe        `json:"-"`
	Results []CheckResult `json:"results"`
	ErrMsg  string        `json:"error,omitempty"`
}

// Add 는 검사 결과 1건을 추가한다.
func (f *FileReport) Add(r CheckResult) { f.Results = append(f.Results, r) }

// Verdict 는 종합 판정 = 가장 심각한 항목의 판정.
func (f *FileReport) Verdict() Severity {
	if f.ErrMsg != "" {
		return Error
	}
	if len(f.Results) == 0 {
		return Skip
	}
	worst := Skip
	for _, r := range f.Results {
		if r.Severity.Rank() > worst.Rank() {
			worst = r.Severity
		}
	}
	return worst
}

// Counts 는 판정별 건수.
func (f *FileReport) Counts() map[string]int {
	c := map[string]int{"SKIP": 0, "PASS": 0, "WARN": 0, "FAIL": 0, "ERROR": 0}
	for _, r := range f.Results {
		c[r.Severity.String()]++
	}
	return c
}

// Failures 는 주의 이상(WARN/FAIL/ERROR) 항목만 추린다.
func (f *FileReport) Failures() []CheckResult {
	var out []CheckResult
	for _, r := range f.Results {
		if r.Severity.Rank() >= Warn.Rank() {
			out = append(out, r)
		}
	}
	return out
}

// BatchReport 는 검사 배치(표본 전체) 결과.
type BatchReport struct {
	Files      []*FileReport  `json:"files"`
	Sampling   map[string]any `json:"sampling"`
	Inventory  []CheckResult  `json:"inventory"`
	SpecMeta   map[string]any `json:"spec_meta"`
	StartedAt  string         `json:"started_at"`
	FinishedAt string         `json:"finished_at"`
}

// Summary 는 배치 요약(파일수·판정분포·합격률).
func (b *BatchReport) Summary() map[string]any {
	total := len(b.Files)
	byVerdict := map[string]int{"SKIP": 0, "PASS": 0, "WARN": 0, "FAIL": 0, "ERROR": 0}
	for _, f := range b.Files {
		byVerdict[f.Verdict().String()]++
	}
	passRate := 0.0
	if total > 0 {
		passRate = float64(byVerdict["PASS"]) / float64(total) * 100
	}
	invIssues := 0
	for _, r := range b.Inventory {
		if r.Severity.Rank() >= Warn.Rank() {
			invIssues++
		}
	}
	return map[string]any{
		"total_files":      total,
		"by_verdict":       byVerdict,
		"pass_rate":        round1(passRate),
		"inventory_issues": invIssues,
	}
}

func round1(v float64) float64 {
	return float64(int(v*10+0.5)) / 10
}
