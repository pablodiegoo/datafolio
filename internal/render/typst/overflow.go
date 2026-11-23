package typst

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/pablodiegoo/datafolio/internal/toolchain"
)

// SlideMarkerSelector is the Typst label attached to start/end metadata markers on every slide.
const SlideMarkerSelector = "<datafolio-slide>"

// SlideOverflow describes a logical slide whose content spilled beyond a single 16:9 PDF page.
type SlideOverflow struct {
	Index     int
	Title     string
	StartPage int
	EndPage   int
	PageSpan  int
	Missing   string
}

// Summary formats a concise human-readable diagnostic naming the slide number, title, and page span.
func (o SlideOverflow) Summary() string {
	label := fmt.Sprintf("slide %d", o.Index)
	if strings.TrimSpace(o.Title) != "" {
		label = fmt.Sprintf("slide %d (%q)", o.Index, o.Title)
	}
	if o.Missing != "" {
		return fmt.Sprintf("slide overflow on %s: missing %s boundary marker", label, o.Missing)
	}
	return fmt.Sprintf("slide overflow on %s: spilled across pages %d-%d (%d pages)", label, o.StartPage, o.EndPage, o.PageSpan)
}

// FormatSlideOverflows renders one or more SlideOverflow records into a single diagnostic string.
func FormatSlideOverflows(overflows []SlideOverflow) string {
	if len(overflows) == 0 {
		return ""
	}
	parts := make([]string, len(overflows))
	for i, ov := range overflows {
		parts[i] = ov.Summary()
	}
	return strings.Join(parts, "; ")
}

type slideMarker struct {
	Kind  string `json:"kind"`
	Index int    `json:"index"`
	Title string `json:"title"`
	Page  int    `json:"page"`
}

type slideMarkerEnvelope struct {
	Value *slideMarker `json:"value"`
	Kind  string       `json:"kind"`
	Index int          `json:"index"`
	Title string       `json:"title"`
	Page  int          `json:"page"`
}

// CheckSlideOverflow queries Typst boundary markers and reports every slide that spilled across
// multiple physical PDF pages or is missing a boundary marker.
func CheckSlideOverflow(ctx context.Context, runner toolchain.TypstRunner, req toolchain.QueryRequest) ([]SlideOverflow, error) {
	if runner == nil {
		return nil, fmt.Errorf("slide overflow check: nil TypstRunner")
	}
	if strings.TrimSpace(req.Selector) == "" {
		req.Selector = SlideMarkerSelector
	}

	raw, err := runner.Query(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("query slide markers: %w", err)
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("query slide markers: empty output from typst query")
	}

	var envelopes []slideMarkerEnvelope
	if err := json.Unmarshal(trimmed, &envelopes); err != nil {
		return nil, fmt.Errorf("parse slide markers JSON: %w", err)
	}

	type slideBounds struct {
		title     string
		hasStart  bool
		startPage int
		hasEnd    bool
		endPage   int
	}
	byIndex := make(map[int]*slideBounds)

	for i, env := range envelopes {
		var m slideMarker
		if env.Value != nil {
			m = *env.Value
		} else {
			m = slideMarker{
				Kind:  env.Kind,
				Index: env.Index,
				Title: env.Title,
				Page:  env.Page,
			}
		}
		if m.Index <= 0 {
			return nil, fmt.Errorf("parse slide marker %d: invalid slide index %d", i, m.Index)
		}
		if m.Kind != "start" && m.Kind != "end" {
			return nil, fmt.Errorf("parse slide marker %d: invalid kind %q", i, m.Kind)
		}

		sb, ok := byIndex[m.Index]
		if !ok {
			sb = &slideBounds{}
			byIndex[m.Index] = sb
		}
		if sb.title == "" && strings.TrimSpace(m.Title) != "" {
			sb.title = strings.TrimSpace(m.Title)
		}
		if m.Kind == "start" {
			if !sb.hasStart || m.Page < sb.startPage {
				sb.startPage = m.Page
				sb.hasStart = true
			}
		} else {
			if !sb.hasEnd || m.Page > sb.endPage {
				sb.endPage = m.Page
				sb.hasEnd = true
			}
		}
	}

	indices := make([]int, 0, len(byIndex))
	for idx := range byIndex {
		indices = append(indices, idx)
	}
	sort.Ints(indices)

	var overflows []SlideOverflow
	for _, idx := range indices {
		sb := byIndex[idx]
		switch {
		case !sb.hasStart:
			overflows = append(overflows, SlideOverflow{
				Index:   idx,
				Title:   sb.title,
				EndPage: sb.endPage,
				Missing: "start",
			})
		case !sb.hasEnd:
			overflows = append(overflows, SlideOverflow{
				Index:     idx,
				Title:     sb.title,
				StartPage: sb.startPage,
				Missing:   "end",
			})
		case sb.endPage > sb.startPage:
			overflows = append(overflows, SlideOverflow{
				Index:     idx,
				Title:     sb.title,
				StartPage: sb.startPage,
				EndPage:   sb.endPage,
				PageSpan:  sb.endPage - sb.startPage + 1,
			})
		}
	}
	return overflows, nil
}
