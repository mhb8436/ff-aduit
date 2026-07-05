package model

import (
	"strconv"
	"strings"
)

// Probe 는 ffprobe -show_format -show_streams -of json 출력을 매핑한다.
type Probe struct {
	Streams []Stream `json:"streams"`
	Format  Format   `json:"format"`
}

// Stream 은 영상/오디오 스트림 메타데이터(필요한 필드만).
type Stream struct {
	Index        int    `json:"index"`
	CodecName    string `json:"codec_name"`
	CodecType    string `json:"codec_type"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	PixFmt       string `json:"pix_fmt"`
	FieldOrder   string `json:"field_order"`
	AvgFrameRate string `json:"avg_frame_rate"`
	RFrameRate   string `json:"r_frame_rate"`
	BitRate      string `json:"bit_rate"`
	StartTime    string `json:"start_time"`
	Channels     int    `json:"channels"`
	SampleRate   string `json:"sample_rate"`
}

// Format 은 컨테이너 레벨 메타데이터.
type Format struct {
	FormatName string `json:"format_name"`
	Duration   string `json:"duration"`
	Size       string `json:"size"`
	BitRate    string `json:"bit_rate"`
}

// FirstStream 은 지정 타입(video/audio)의 첫 스트림을 반환.
func (p *Probe) FirstStream(codecType string) *Stream {
	for i := range p.Streams {
		if p.Streams[i].CodecType == codecType {
			return &p.Streams[i]
		}
	}
	return nil
}

// DurationSec 은 컨테이너 재생시간(초).
func (p *Probe) DurationSec() float64 {
	return ParseFloat(p.Format.Duration)
}

// ParseFloat 은 실패 시 0 을 반환하는 관대한 파서.
func ParseFloat(s string) float64 {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0
	}
	return v
}

// ParseInt 은 실패 시 0 을 반환.
func ParseInt(s string) int {
	f := ParseFloat(s)
	return int(f)
}

// ParseRate 는 "30000/1001" 형태의 프레임레이트를 float 으로 변환.
func ParseRate(rate string) float64 {
	rate = strings.TrimSpace(rate)
	if rate == "" {
		return 0
	}
	if strings.Contains(rate, "/") {
		parts := strings.SplitN(rate, "/", 2)
		num := ParseFloat(parts[0])
		den := ParseFloat(parts[1])
		if den == 0 {
			return 0
		}
		return round3(num / den)
	}
	return ParseFloat(rate)
}

func round3(v float64) float64 {
	return float64(int(v*1000+0.5)) / 1000
}

// ProbeSummary 는 리포트 표시용 핵심 기술정보 요약을 만든다.
func (p *Probe) Summary() map[string]any {
	if p == nil {
		return map[string]any{}
	}
	out := map[string]any{
		"container":    p.Format.FormatName,
		"duration_sec": p.DurationSec(),
		"size_bytes":   ParseInt(p.Format.Size),
		"bitrate_bps":  ParseInt(p.Format.BitRate),
	}
	if v := p.FirstStream("video"); v != nil {
		fr := v.AvgFrameRate
		if fr == "" || fr == "0/0" {
			fr = v.RFrameRate
		}
		out["video"] = map[string]any{
			"codec":       v.CodecName,
			"width":       v.Width,
			"height":      v.Height,
			"frame_rate":  ParseRate(fr),
			"pix_fmt":     v.PixFmt,
			"field_order": v.FieldOrder,
		}
	}
	if a := p.FirstStream("audio"); a != nil {
		out["audio"] = map[string]any{
			"codec":       a.CodecName,
			"channels":    a.Channels,
			"sample_rate": ParseInt(a.SampleRate),
		}
	}
	return out
}

// MarshalJSON 커스텀: FileReport 에 probe_summary/verdict/counts 를 포함.
func (f *FileReport) toMap() map[string]any {
	var summary map[string]any
	if f.Probe != nil {
		summary = f.Probe.Summary()
	}
	return map[string]any{
		"path":          f.Path,
		"profile":       f.Profile,
		"verdict":       f.Verdict().String(),
		"counts":        f.Counts(),
		"error":         f.ErrMsg,
		"probe_summary": summary,
		"results":       f.Results,
	}
}
