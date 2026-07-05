// Package spec 는 검사 기준(spec.yaml)을 로드하고 프로파일을 라우팅한다.
//
// 감리는 "발주기관이 정한 납품규격 대비 준수 여부"를 점검하므로,
// 기준값은 코드가 아닌 YAML 로 외부화한다. 기본 기준은 exe 에 내장(embed)되며,
// --spec 로 외부 파일을 지정하면 그것을 우선한다.
package spec

import (
	_ "embed"
	"os"
	"regexp"

	"gopkg.in/yaml.v3"
)

//go:embed spec.default.yaml
var defaultYAML []byte

// Spec 은 로드된 기준 전체를 보관한다.
type Spec struct {
	data map[string]any
}

// Load 는 경로가 비어 있으면 내장 기본기준을, 아니면 파일을 로드한다.
func Load(path string) (*Spec, error) {
	var raw []byte
	if path == "" {
		raw = defaultYAML
	} else {
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		raw = b
	}
	var m map[string]any
	if err := yaml.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	return &Spec{data: m}, nil
}

// ---- 섹션 접근 (모두 map[string]any 로 반환) ----

func (s *Spec) Meta() map[string]any     { return s.section("meta") }
func (s *Spec) Quality() map[string]any  { return s.section("quality") }
func (s *Spec) Naming() map[string]any   { return s.section("naming") }
func (s *Spec) Inventory() map[string]any { return s.section("inventory") }
func (s *Spec) Sampling() map[string]any { return s.section("sampling") }

func (s *Spec) section(key string) map[string]any {
	if v, ok := s.data[key].(map[string]any); ok {
		return v
	}
	return map[string]any{}
}

// ProfileFor 는 파일 상대경로에 맞는 프로파일 이름과 정의를 라우팅 규칙으로 결정.
func (s *Spec) ProfileFor(relPath string) (string, map[string]any) {
	routing := s.section("routing")
	if rules, ok := routing["rules"].([]any); ok {
		for _, r := range rules {
			rm, ok := r.(map[string]any)
			if !ok {
				continue
			}
			pat, _ := rm["pattern"].(string)
			prof, _ := rm["profile"].(string)
			if pat != "" {
				if re, err := regexp.Compile(pat); err == nil && re.MatchString(relPath) {
					return prof, s.Profile(prof)
				}
			}
		}
	}
	def, _ := routing["default_profile"].(string)
	if def == "" {
		// profiles 의 첫 키
		if profs, ok := s.data["profiles"].(map[string]any); ok {
			for k := range profs {
				def = k
				break
			}
		}
	}
	return def, s.Profile(def)
}

// Profile 은 이름으로 프로파일 정의를 반환.
func (s *Spec) Profile(name string) map[string]any {
	if profs, ok := s.data["profiles"].(map[string]any); ok {
		if p, ok := profs[name].(map[string]any); ok {
			return p
		}
	}
	return map[string]any{}
}
