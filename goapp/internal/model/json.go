package model

import "encoding/json"

// ToDict 는 리포트 JSON 직렬화용 맵을 만든다(파일별 요약·판정 포함).
func (b *BatchReport) ToDict() map[string]any {
	files := make([]map[string]any, 0, len(b.Files))
	for _, f := range b.Files {
		files = append(files, f.toMap())
	}
	return map[string]any{
		"spec_meta":   b.SpecMeta,
		"started_at":  b.StartedAt,
		"finished_at": b.FinishedAt,
		"sampling":    b.Sampling,
		"summary":     b.Summary(),
		"inventory":   b.Inventory,
		"files":       files,
	}
}

// ToJSON 은 들여쓴 JSON 문자열을 반환.
func (b *BatchReport) ToJSON() (string, error) {
	buf, err := json.MarshalIndent(b.ToDict(), "", "  ")
	if err != nil {
		return "", err
	}
	return string(buf), nil
}

// FileSummary 는 report 패키지에서 파일 요약 맵이 필요할 때 사용.
func (f *FileReport) FileSummary() map[string]any { return f.toMap() }
