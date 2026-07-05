// vqc — 방송자료 디지털화 영상 검사(Video QC) 감리 도구 (Go 구현)
//
// 사용 예)
//
//	vqc inspect <폴더|파일> --deep --inventory 납품목록.csv --report out
//	vqc plan <폴더>            # 표본 선정 미리보기
//	vqc probe <파일>           # 기술 메타데이터 출력
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"vqc/internal/engine"
	"vqc/internal/model"
	"vqc/internal/report"
	"vqc/internal/sampling"
	"vqc/internal/spec"
	"vqc/internal/tools"
)

const version = "1.0.0"

// parseInterspersed 는 플래그와 위치인자가 섞여 있어도 모두 파싱한다.
// Go flag 패키지는 첫 위치인자에서 멈추므로, 남은 인자를 반복 파싱하여
// `inspect <대상> --deep` 처럼 대상이 앞에 와도 플래그가 적용되게 한다.
func parseInterspersed(fs *flag.FlagSet, args []string) []string {
	var positional []string
	rest := args
	for {
		if err := fs.Parse(rest); err != nil {
			return positional
		}
		if fs.NArg() == 0 {
			break
		}
		positional = append(positional, fs.Arg(0))
		rest = fs.Args()[1:]
	}
	return positional
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "inspect":
		os.Exit(cmdInspect(os.Args[2:]))
	case "plan":
		os.Exit(cmdPlan(os.Args[2:]))
	case "probe":
		os.Exit(cmdProbe(os.Args[2:]))
	case "--version", "version", "-v":
		fmt.Println("vqc", version)
	case "-h", "--help", "help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "알 수 없는 명령: %s\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Println(`vqc — 방송자료 디지털화 영상 검사 감리 도구

사용법:
  vqc inspect <폴더|파일> [옵션]   영상 검사 실행
  vqc plan    <폴더>              표본 선정 미리보기
  vqc probe   <파일>              기술 메타데이터 출력
  vqc --version

inspect 옵션:
  --deep                심층검사(전체 디코드·화질/음질 결함검출)
  --no-sample           표본검수 없이 전수검사
  --ratio <float>       표본 비율 강제(예: 0.1)
  --inventory <csv>     납품목록 CSV 대조
  --report <dir>        리포트 출력 디렉토리(html/csv/json)
  --spec <yaml>         검사기준 파일(기본: 내장 기준)
  --seed <str>          표본 재현용 시드(기본 kpf-2026)`)
}

func cmdInspect(args []string) int {
	fs := flag.NewFlagSet("inspect", flag.ExitOnError)
	deep := fs.Bool("deep", false, "심층검사")
	noSample := fs.Bool("no-sample", false, "전수검사")
	ratio := fs.Float64("ratio", 0, "표본 비율 강제")
	inv := fs.String("inventory", "", "납품목록 CSV")
	rep := fs.String("report", "", "리포트 디렉토리")
	specPath := fs.String("spec", "", "검사기준 YAML")
	seed := fs.String("seed", "kpf-2026", "표본 시드")
	pos := parseInterspersed(fs, args)
	if len(pos) < 1 {
		fmt.Fprintln(os.Stderr, "검사 대상(폴더|파일)을 지정하세요.")
		return 2
	}
	target := pos[0]

	tl, err := tools.Default()
	if err != nil {
		fmt.Fprintf(os.Stderr, "[오류] %v\n", err)
		return 2
	}
	sp, err := spec.Load(*specPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[오류] 검사기준 로드 실패: %v\n", err)
		return 2
	}

	fmt.Printf("영상 검사 시작: %s\n", target)
	batch := engine.Run(target, sp, tl, engine.Options{
		Deep: *deep, Sample: !*noSample, InventoryCSV: *inv,
		SampleRatio: *ratio, Seed: *seed,
		Progress: func(i, n int, p string) {
			fmt.Printf("  [%d/%d] 검사 중: %s\n", i, n, filepath.Base(p))
		},
	})
	if len(batch.Files) == 0 {
		fmt.Fprintln(os.Stderr, "[경고] 검사할 영상 파일을 찾지 못했습니다.")
		return 1
	}

	report.PrintConsoleSummary(batch)

	if *rep != "" {
		if err := os.MkdirAll(*rep, 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "[오류] 리포트 디렉토리 생성 실패: %v\n", err)
			return 2
		}
		h := filepath.Join(*rep, "vqc_report.html")
		x := filepath.Join(*rep, "vqc_report.xlsx")
		j := filepath.Join(*rep, "vqc_report.json")
		report.WriteHTML(batch, h)
		if err := report.WriteXLSX(batch, x); err != nil {
			fmt.Fprintf(os.Stderr, "[경고] 엑셀 리포트 생성 실패: %v\n", err)
		}
		report.WriteJSON(batch, j)
		fmt.Printf("리포트 생성:\n  %s\n  %s\n  %s\n", x, h, j)
	}

	// 종료코드: FAIL/ERROR 있으면 1
	worst := 0
	for _, f := range batch.Files {
		if f.Verdict().Rank() > worst {
			worst = f.Verdict().Rank()
		}
	}
	for _, r := range batch.Inventory {
		if r.Severity.Rank() > worst {
			worst = r.Severity.Rank()
		}
	}
	if worst >= model.Fail.Rank() {
		return 1
	}
	return 0
}

func cmdPlan(args []string) int {
	fs := flag.NewFlagSet("plan", flag.ExitOnError)
	ratio := fs.Float64("ratio", 0, "표본 비율")
	seed := fs.String("seed", "kpf-2026", "표본 시드")
	specPath := fs.String("spec", "", "검사기준 YAML")
	pos := parseInterspersed(fs, args)
	if len(pos) < 1 {
		fmt.Fprintln(os.Stderr, "대상 폴더를 지정하세요.")
		return 2
	}
	sp, err := spec.Load(*specPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[오류] %v\n", err)
		return 2
	}
	files := engine.CollectFiles(pos[0])
	if len(files) == 0 {
		fmt.Fprintln(os.Stderr, "영상 파일 없음.")
		return 1
	}
	base := commonBase(files)
	items := make([]sampling.Item, len(files))
	for i, p := range files {
		rel, _ := filepath.Rel(base, p)
		pn, _ := sp.ProfileFor(rel)
		items[i] = sampling.Item{Path: p, Profile: pn}
	}
	plan := sampling.PlanSample(items, sp, *seed, *ratio)
	fmt.Printf("모집단: %d개\n", plan.Population)
	fmt.Printf("표본 방식: %s   표본 수: %d개\n", plan.Method, plan.Size)
	fmt.Printf("파라미터: %v\n", plan.Params)
	if len(plan.Strata) > 0 {
		fmt.Println("층별 배분:")
		sk := make([]string, 0, len(plan.Strata))
		for k := range plan.Strata {
			sk = append(sk, k)
		}
		sort.Strings(sk)
		for _, k := range sk {
			fmt.Printf("  %s: 모집단 %d → 표본 %d\n", k, plan.Strata[k]["population"], plan.Strata[k]["sampled"])
		}
	}
	fmt.Println("\n선정된 표본:")
	for _, p := range plan.Selected {
		rel, _ := filepath.Rel(base, p)
		fmt.Printf("  %s\n", rel)
	}
	return 0
}

func cmdProbe(args []string) int {
	fs := flag.NewFlagSet("probe", flag.ExitOnError)
	pos := parseInterspersed(fs, args)
	if len(pos) < 1 {
		fmt.Fprintln(os.Stderr, "대상 파일을 지정하세요.")
		return 2
	}
	tl, err := tools.Default()
	if err != nil {
		fmt.Fprintf(os.Stderr, "[오류] %v\n", err)
		return 2
	}
	p, err := tl.RunProbeJSON(pos[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "[오류] %v\n", err)
		return 2
	}
	buf, _ := json.MarshalIndent(p.Summary(), "", "  ")
	fmt.Println(string(buf))
	return 0
}

func commonBase(paths []string) string {
	if len(paths) == 1 {
		return filepath.Dir(paths[0])
	}
	base := filepath.Dir(paths[0])
	for _, p := range paths[1:] {
		d := filepath.Dir(p)
		for !isPrefix(base, d) {
			parent := filepath.Dir(base)
			if parent == base {
				break
			}
			base = parent
		}
	}
	return base
}

func isPrefix(base, path string) bool {
	rel, err := filepath.Rel(base, path)
	return err == nil && rel != ".." && !hasDotDotPrefix(rel)
}

func hasDotDotPrefix(rel string) bool {
	return len(rel) >= 2 && rel[0] == '.' && rel[1] == '.'
}
