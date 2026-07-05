//go:build !windows

// 비-Windows 빌드(맥/리눅스). ffmpeg 를 내장하지 않고 시스템 PATH 의
// ffmpeg 를 사용한다 → 개발·검증 환경에서 동일 로직을 그대로 테스트한다.
package tools

// extractEmbedded 는 내장본이 없음을 알려 상위 resolve 가 PATH 로 폴백하게 한다.
func extractEmbedded(name string) (string, bool, error) {
	return "", false, nil
}
