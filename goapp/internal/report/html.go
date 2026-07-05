package report

import (
	"fmt"
	"html"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"vqc/internal/model"
)

var sevColor = map[string]string{
	"PASS": "#1a7f37", "WARN": "#9a6700", "FAIL": "#cf222e",
	"ERROR": "#82071e", "SKIP": "#57606a",
}

// WriteHTML 은 감리 증적용 HTML 리포트를 저장한다.
func WriteHTML(b *model.BatchReport, path string) error {
	return os.WriteFile(path, []byte(renderHTML(b)), 0o644)
}

func badge(sev string) string {
	c := sevColor[sev]
	if c == "" {
		c = "#57606a"
	}
	return fmt.Sprintf(`<span style="display:inline-block;padding:1px 8px;border-radius:10px;font-size:12px;font-weight:600;color:#fff;background:%s">%s</span>`, c, sev)
}

func renderHTML(b *model.BatchReport) string {
	var w strings.Builder
	s := b.Summary()
	meta := b.SpecMeta
	proj, _ := meta["project"].(string)
	auth, _ := meta["authority"].(string)
	specVer := fmt.Sprintf("%v", meta["spec_version"])

	w.WriteString("<!doctype html><html lang='ko'><head><meta charset='utf-8'>")
	w.WriteString("<meta name='viewport' content='width=device-width, initial-scale=1'>")
	fmt.Fprintf(&w, "<title>영상 검사 결과 — %s</title>", html.EscapeString(proj))
	w.WriteString(cssBlock)
	w.WriteString("</head><body><div class='wrap'>")

	// 헤더
	w.WriteString("<h1>영상 납품자료 검사 결과 보고</h1>")
	fmt.Fprintf(&w, "<div class='sub'>%s · %s · 검사기준 %s</div>",
		html.EscapeString(proj), html.EscapeString(auth), html.EscapeString(specVer))
	fmt.Fprintf(&w, "<div class='sub'>검사일시(UTC): %s ~ %s</div>",
		html.EscapeString(b.StartedAt), html.EscapeString(b.FinishedAt))

	// 요약 카드
	byV, _ := s["by_verdict"].(map[string]int)
	w.WriteString("<div class='cards'>")
	fmt.Fprintf(&w, "<div class='card'><div class='n'>%v</div><div class='l'>검사 파일</div></div>", s["total_files"])
	for _, sev := range []string{"PASS", "WARN", "FAIL", "ERROR"} {
		fmt.Fprintf(&w, "<div class='card'><div class='n' style='color:%s'>%d</div><div class='l'>%s</div></div>",
			sevColor[sev], byV[sev], sev)
	}
	fmt.Fprintf(&w, "<div class='card'><div class='n'>%v%%</div><div class='l'>합격률</div></div>", s["pass_rate"])
	w.WriteString("</div>")

	// 표본 정보
	w.WriteString("<h2>표본검수 정보</h2>")
	w.WriteString("<table><tr><th>방식</th><th>모집단</th><th>표본수</th><th>파라미터</th></tr>")
	smp := b.Sampling
	paramStr := ""
	if params, ok := smp["params"].(map[string]any); ok {
		var kv []string
		pk := make([]string, 0, len(params))
		for k := range params {
			pk = append(pk, k)
		}
		sort.Strings(pk)
		for _, k := range pk {
			kv = append(kv, fmt.Sprintf("%s=%v", k, params[k]))
		}
		paramStr = strings.Join(kv, ", ")
	}
	fmt.Fprintf(&w, "<tr><td>%v</td><td>%v</td><td>%v</td><td class='muted'>%s</td></tr></table>",
		smp["method"], smp["population"], firstNonNil(smp["selected_count"], smp["size"]),
		html.EscapeString(paramStr))

	// 납품목록 정합성
	if len(b.Inventory) > 0 {
		w.WriteString("<h2>납품목록·정합성 점검(배치 단위)</h2>")
		w.WriteString("<table><tr><th>항목</th><th>판정</th><th>사유</th></tr>")
		for _, r := range b.Inventory {
			fmt.Fprintf(&w, "<tr><td>%s</td><td>%s</td><td>%s</td></tr>",
				html.EscapeString(r.Title), badge(r.Severity.String()), html.EscapeString(r.Message))
		}
		w.WriteString("</table>")
	}

	// 파일별 상세
	w.WriteString("<h2>파일별 검사 결과</h2>")
	for _, fr := range b.Files {
		v := fr.Verdict().String()
		tech := ""
		durs := ""
		if fr.Probe != nil {
			ps := fr.Probe.Summary()
			if vinfo, ok := ps["video"].(map[string]any); ok {
				tech = fmt.Sprintf("%v %vx%v %vfps", vinfo["codec"], vinfo["width"], vinfo["height"], vinfo["frame_rate"])
			}
			if d, ok := ps["duration_sec"].(float64); ok && d > 0 {
				durs = fmt.Sprintf("%.0fs", d)
			}
		}
		w.WriteString("<details><summary>")
		fmt.Fprintf(&w, "<div class='file-hd'><div><b>%s</b> <span class='muted mono'>[%s] %s %s</span></div><div>%s</div></div></summary>",
			html.EscapeString(filepath.Base(fr.Path)), html.EscapeString(fr.Profile),
			html.EscapeString(tech), durs, badge(v))
		w.WriteString("<table><tr><th>분류</th><th>점검항목</th><th>판정</th><th>기대값</th><th>실측값</th><th>판정사유</th></tr>")
		for _, r := range fr.Results {
			fmt.Fprintf(&w, "<tr><td class='muted'>%s</td><td>%s</td><td>%s</td><td class='mono muted'>%s</td><td class='mono'>%s</td><td>%s</td></tr>",
				html.EscapeString(r.Category), html.EscapeString(r.Title), badge(r.Severity.String()),
				html.EscapeString(short(r.Expected)), html.EscapeString(short(r.Actual)), html.EscapeString(r.Message))
		}
		w.WriteString("</table></details>")
	}

	w.WriteString("<p class='sub' style='margin-top:24px'>본 리포트는 vqc(영상 검사 감리 도구)로 자동 생성되었습니다. 판정(WARN/FAIL/ERROR) 항목은 증적과 함께 감리수행결과보고서에 반영됩니다.</p>")
	w.WriteString("</div></body></html>")
	return w.String()
}

const cssBlock = `<style>
body{font-family:-apple-system,'Apple SD Gothic Neo',Malgun Gothic,sans-serif;margin:0;color:#1f2328;background:#f6f8fa}
.wrap{max-width:1100px;margin:0 auto;padding:24px}
h1{font-size:22px;margin:0 0 4px} h2{font-size:17px;margin:28px 0 10px}
.sub{color:#57606a;font-size:13px}
.cards{display:flex;gap:12px;flex-wrap:wrap;margin:16px 0}
.card{background:#fff;border:1px solid #d0d7de;border-radius:10px;padding:14px 18px;min-width:110px}
.card .n{font-size:26px;font-weight:700} .card .l{font-size:12px;color:#57606a}
table{border-collapse:collapse;width:100%;background:#fff;font-size:13px;border:1px solid #d0d7de;border-radius:8px;overflow:hidden}
th,td{padding:7px 10px;text-align:left;border-bottom:1px solid #eaeef2;vertical-align:top}
th{background:#f6f8fa;font-weight:600}
tr:last-child td{border-bottom:none}
.file-hd{cursor:pointer;background:#fff;border:1px solid #d0d7de;border-radius:8px;padding:10px 14px;margin:8px 0;display:flex;justify-content:space-between;align-items:center}
.mono{font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-size:12px}
details>summary{list-style:none;cursor:pointer}
details>summary::-webkit-details-marker{display:none}
.muted{color:#57606a}
</style>`
