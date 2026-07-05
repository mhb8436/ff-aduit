// Package engine 은 검사 전 과정을 오케스트레이션한다.
// 파일 수집 → 표본선정 → 파일별 검사 → 납품목록 대조 → 배치 리포트.
package engine

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"vqc/internal/checks"
	"vqc/internal/model"
	"vqc/internal/sampling"
	"vqc/internal/spec"
	"vqc/internal/tools"
)

var videoExts = map[string]bool{
	".mxf": true, ".mov": true, ".mkv": true, ".avi": true, ".mp4": true,
	".m4v": true, ".mpg": true, ".mpeg": true, ".ts": true, ".m2ts": true,
	".wmv": true, ".vob": true, ".dv": true,
}

// Options 는 검사 실행 옵션.
type Options struct {
	Deep         bool
	Sample       bool
	InventoryCSV string
	MetadataCSV  string
	SampleRatio  float64
	Seed         string
	Progress     func(i, n int, path string)
}

// CollectFiles 는 파일/디렉토리에서 영상 파일 경로를 수집한다.
func CollectFiles(target string) []string {
	fi, err := os.Stat(target)
	if err != nil {
		return nil
	}
	if !fi.IsDir() {
		abs, _ := filepath.Abs(target)
		return []string{abs}
	}
	var found []string
	filepath.Walk(target, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if videoExts[strings.ToLower(filepath.Ext(p))] {
			abs, _ := filepath.Abs(p)
			found = append(found, abs)
		}
		return nil
	})
	sort.Strings(found)
	return found
}

// InspectFile 은 파일 1개를 전체 검사한다.
func InspectFile(path string, sp *spec.Spec, tl tools.Tools, baseDir string, deep bool) *model.FileReport {
	fr := &model.FileReport{Path: path}
	rel := path
	if baseDir != "" {
		if r, err := filepath.Rel(baseDir, path); err == nil {
			rel = r
		}
	}
	profName, _ := sp.ProfileFor(rel)
	fr.Profile = profName
	profile := sp.Profile(profName)

	probe, err := tl.RunProbeJSON(path)
	if err != nil {
		fr.ErrMsg = "파일 분석 실패(재생 불가 가능성): " + err.Error()
		fr.Add(model.CheckResult{CheckID: "integrity.open", Category: "무결성/재생가능",
			Title: "파일 열기", Severity: model.Error, Message: fr.ErrMsg})
		return fr
	}
	fr.Probe = probe

	checks.Integrity(fr, probe, sp, tl, deep)
	checks.TechSpec(fr, probe, profile, sp, tl)
	checks.VideoDefect(fr, probe, sp, tl, deep)
	checks.AudioDefect(fr, probe, sp, tl, deep)
	checks.AVSync(fr, probe, sp)
	checks.FileNaming(fr, sp, baseDir)
	return fr
}

// Run 은 전체 검사를 실행한다.
func Run(target string, sp *spec.Spec, tl tools.Tools, opt Options) *model.BatchReport {
	batch := &model.BatchReport{}
	batch.SpecMeta = sp.Meta()
	batch.StartedAt = time.Now().UTC().Format(time.RFC3339)

	all := CollectFiles(target)
	if len(all) == 0 {
		batch.FinishedAt = time.Now().UTC().Format(time.RFC3339)
		return batch
	}

	baseDir := commonBase(all)

	var targets []string
	if opt.Sample {
		items := make([]sampling.Item, len(all))
		for i, p := range all {
			rel, _ := filepath.Rel(baseDir, p)
			pn, _ := sp.ProfileFor(rel)
			items[i] = sampling.Item{Path: p, Profile: pn}
		}
		plan := sampling.PlanSample(items, sp, opt.Seed, opt.SampleRatio)
		targets = plan.Selected
		batch.Sampling = plan.ToMap()
		batch.Sampling["selected_count"] = len(targets)
	} else {
		targets = all
		batch.Sampling = map[string]any{"method": "full", "population": len(all),
			"size": len(all), "selected_count": len(all)}
	}

	for i, p := range targets {
		if opt.Progress != nil {
			opt.Progress(i+1, len(targets), p)
		}
		batch.Files = append(batch.Files, InspectFile(p, sp, tl, baseDir, opt.Deep))
	}

	batch.Inventory = checks.Inventory(opt.InventoryCSV, batch.Files, sp, baseDir)
	batch.Inventory = append(batch.Inventory,
		checks.Metadata(opt.MetadataCSV, opt.InventoryCSV, batch.Files, sp)...)
	batch.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	return batch
}

func commonBase(paths []string) string {
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
