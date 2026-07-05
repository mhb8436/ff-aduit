package tools

import (
	"bytes"
	"encoding/json"
	"os/exec"

	"vqc/internal/model"
)

// runCapture 는 stdout 을 반환(오류 시 error).
func runCapture(bin string, args ...string) ([]byte, error) {
	cmd := exec.Command(bin, args...)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return out.Bytes(), err
	}
	return out.Bytes(), nil
}

// runFull 은 stdout, stderr, error 를 모두 반환.
func runFull(bin string, args ...string) (string, string, error) {
	cmd := exec.Command(bin, args...)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	err := cmd.Run()
	return out.String(), errb.String(), err
}

func parseProbe(b []byte) (*model.Probe, error) {
	var p model.Probe
	if err := json.Unmarshal(b, &p); err != nil {
		return nil, err
	}
	return &p, nil
}
