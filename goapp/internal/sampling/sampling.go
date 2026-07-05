// Package sampling 은 표본검수(샘플링) 대상 선정을 담당한다.
//
// RFP 요구: "실제 구축자료를 표본 추출하여 실증적 검증 ... 표본검수 방식과
// 표본 선정 기준을 감리계획서에 명시". (DB 검수 8% 이상 샘플링)
//
// 통계적 표본크기(Cochran + 유한모집단 보정)를 산출하고, 층화(방송사/매체/프로파일)
// 기준으로 배분하여 고정 시드 해시로 재현 가능하게 선정한다.
package sampling

import (
	"crypto/sha256"
	"encoding/hex"
	"math"
	"path/filepath"
	"sort"
	"strings"

	"vqc/internal/spec"
)

// Item 은 모집단의 파일 1건.
type Item struct {
	Path    string
	Profile string
}

// Plan 은 표본 선정 결과.
type Plan struct {
	Method     string
	Population int
	Size       int
	Target     int
	Selected   []string
	Params     map[string]any
	Strata     map[string]map[string]int
}

// ToMap 은 리포트 직렬화용(선정 목록 제외).
func (p *Plan) ToMap() map[string]any {
	return map[string]any{
		"method":     p.Method,
		"population": p.Population,
		"size":       p.Size,
		"target":     p.Target,
		"params":     p.Params,
		"strata":     p.Strata,
	}
}

// CochranSampleSize 는 Cochran 공식 + 유한모집단 보정으로 표본 크기를 산출.
func CochranSampleSize(N int, confidence, margin, p float64) int {
	z := 1.96
	switch round2(confidence) {
	case 0.90:
		z = 1.645
	case 0.95:
		z = 1.96
	case 0.99:
		z = 2.576
	}
	if N <= 0 {
		return 0
	}
	n0 := (z * z * p * (1 - p)) / (margin * margin)
	n := n0 / (1 + (n0-1)/float64(N))
	return int(math.Ceil(n))
}

// PlanSample 은 표본을 선정한다.
func PlanSample(items []Item, sp *spec.Spec, seed string, overrideRatio float64) *Plan {
	cfg := sp.Sampling()
	N := len(items)
	confidence, _ := spec.Float(cfg, "confidence_level", 0.95)
	margin, _ := spec.Float(cfg, "margin_of_error", 0.05)
	minRatio, _ := spec.Float(cfg, "min_ratio", 0.08)
	if overrideRatio > 0 {
		minRatio = overrideRatio
	}
	minPerStratum := spec.Int(cfg, "min_per_stratum", 2)
	fullBelow := spec.Int(cfg, "full_inspect_below", 30)

	paths := make([]string, len(items))
	for i, it := range items {
		paths[i] = it.Path
	}
	baseDir := commonDir(paths)

	// 전수검사 조건
	if N <= fullBelow {
		sel := make([]string, len(items))
		for i, it := range items {
			sel[i] = it.Path
		}
		return &Plan{
			Method: "full", Population: N, Size: N, Target: N, Selected: sel,
			Params: map[string]any{"reason": firstNonEmpty(
				sprintfBelow(N, fullBelow))},
			Strata: map[string]map[string]int{},
		}
	}

	nStat := CochranSampleSize(N, confidence, margin, 0.5)
	nRatio := int(math.Ceil(float64(N) * minRatio))
	target := nStat
	if nRatio > target {
		target = nRatio
	}
	if target > N {
		target = N
	}

	// 층화 배분
	strata := map[string][]string{}
	for _, it := range items {
		key := stratumKey(it.Path, baseDir, it.Profile)
		strata[key] = append(strata[key], it.Path)
	}

	selectedSet := map[string]bool{}
	strataReport := map[string]map[string]int{}
	for key, ps := range strata {
		share := int(math.Round(float64(target) * float64(len(ps)) / float64(N)))
		if share < minPerStratum {
			share = minPerStratum
		}
		if share > len(ps) {
			share = len(ps)
		}
		ordered := make([]string, len(ps))
		copy(ordered, ps)
		sort.Slice(ordered, func(i, j int) bool {
			return stableRank(ordered[i], seed) < stableRank(ordered[j], seed)
		})
		chosen := ordered[:share]
		for _, c := range chosen {
			selectedSet[c] = true
		}
		strataReport[key] = map[string]int{"population": len(ps), "sampled": len(chosen)}
	}

	selected := make([]string, 0, len(selectedSet))
	for s := range selectedSet {
		selected = append(selected, s)
	}
	sort.Slice(selected, func(i, j int) bool {
		return stableRank(selected[i], seed) < stableRank(selected[j], seed)
	})

	return &Plan{
		Method: "stratified", Population: N, Size: len(selected), Target: target,
		Selected: selected,
		Params: map[string]any{
			"confidence_level": confidence, "margin_of_error": margin,
			"min_ratio": minRatio, "n_statistical": nStat, "n_ratio": nRatio,
			"min_per_stratum": minPerStratum, "seed": seed,
		},
		Strata: strataReport,
	}
}

func stratumKey(path, baseDir, profile string) string {
	rel := path
	if baseDir != "" {
		if r, err := filepath.Rel(baseDir, path); err == nil {
			rel = r
		}
	}
	top := "_root"
	if i := strings.IndexRune(rel, filepath.Separator); i >= 0 {
		top = rel[:i]
	}
	return top + "::" + profile
}

func stableRank(path, seed string) string {
	h := sha256.Sum256([]byte(seed + ":" + path))
	return hex.EncodeToString(h[:])
}

func commonDir(paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	if len(paths) == 1 {
		return filepath.Dir(paths[0])
	}
	parts := strings.Split(filepath.Dir(paths[0]), string(filepath.Separator))
	for _, p := range paths[1:] {
		cur := strings.Split(filepath.Dir(p), string(filepath.Separator))
		n := len(parts)
		if len(cur) < n {
			n = len(cur)
		}
		i := 0
		for i < n && parts[i] == cur[i] {
			i++
		}
		parts = parts[:i]
	}
	return strings.Join(parts, string(filepath.Separator))
}

func round2(v float64) float64 { return float64(int(v*100+0.5)) / 100 }

func firstNonEmpty(s string) string { return s }

func sprintfBelow(n, below int) string {
	return "모집단 " + itoa(n) + " <= " + itoa(below) + " → 전수검사"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}
