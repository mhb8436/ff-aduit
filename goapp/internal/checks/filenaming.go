package checks

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"vqc/internal/model"
	"vqc/internal/spec"
)

const catNaming = "파일명/구조"

// FileNaming 은 파일명/폴더구조 정합성(파일 단위)을 점검한다.
// RFP 중점점검: "파일명, 폴더 구조, 납품자료 목록의 정합성".
func FileNaming(fr *model.FileReport, sp *spec.Spec, baseDir string) {
	naming := sp.Naming()
	fname := filepath.Base(fr.Path)
	rel := fr.Path
	if baseDir != "" {
		if r, err := filepath.Rel(baseDir, fr.Path); err == nil {
			rel = r
		}
	}

	// 1) 확장자
	if allowedExt := spec.StrList(naming, "allowed_extensions"); len(allowedExt) > 0 {
		ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(fname)), ".")
		ok := inList(ext, allowedExt)
		sev := model.Fail
		msg := fmt.Sprintf("허용되지 않은 확장자: .%s (허용 %v).", ext, allowedExt)
		if ok {
			sev, msg = model.Pass, fmt.Sprintf("확장자 적합: .%s", ext)
		}
		fr.Add(model.CheckResult{CheckID: "naming.extension", Category: catNaming,
			Title: "확장자", Severity: sev, Message: msg, Expected: allowedExt, Actual: ext})
	}

	// 2) 금지 문자
	if forbidden := spec.Str(naming, "forbidden_chars"); forbidden != "" {
		var bad []string
		seen := map[rune]bool{}
		for _, c := range fname {
			if strings.ContainsRune(forbidden, c) && !seen[c] {
				bad = append(bad, string(c))
				seen[c] = true
			}
		}
		sort.Strings(bad)
		sev := model.Pass
		msg := "파일명에 금지문자 없음."
		if len(bad) > 0 {
			sev = model.Fail
			msg = fmt.Sprintf("파일명에 금지문자 포함: %v. 시스템 호환/자동처리 오류 우려.", bad)
		}
		fr.Add(model.CheckResult{CheckID: "naming.forbidden_chars", Category: catNaming,
			Title: "금지문자", Severity: sev, Message: msg, Actual: bad})
	}

	// 3) 명명규칙(정규식)
	if pattern := spec.Str(naming, "filename_pattern"); pattern != "" {
		ok := false
		if re, err := regexp.Compile(pattern); err == nil {
			ok = re.MatchString(fname)
		}
		sev := model.Warn
		msg := fmt.Sprintf("명명규칙 불일치: '%s'. 납품 명명체계 확인 필요.", fname)
		if ok {
			sev, msg = model.Pass, "명명규칙 준수."
		}
		fr.Add(model.CheckResult{CheckID: "naming.pattern", Category: catNaming,
			Title: "명명규칙", Severity: sev, Message: msg, Expected: pattern, Actual: fname})
	}

	// 4) 경로 길이
	if maxLen := spec.Int(naming, "max_path_length", 0); maxLen > 0 {
		n := utf8.RuneCountInString(rel)
		ok := n <= maxLen
		sev := model.Pass
		msg := fmt.Sprintf("경로 길이 적정(%d자).", n)
		if !ok {
			sev = model.Warn
			msg = fmt.Sprintf("경로가 김(%d자 > %d). 매체 이관 시 오류 우려.", n, maxLen)
		}
		fr.Add(model.CheckResult{CheckID: "naming.path_length", Category: catNaming,
			Title: "경로 길이", Severity: sev, Message: msg,
			Expected: fmt.Sprintf("<= %d", maxLen), Actual: n})
	}
}
