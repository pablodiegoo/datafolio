package typst_test

import (
	"context"
	"strings"
	"testing"

	"github.com/pablodiegoo/datafolio/internal/render/typst"
	"github.com/pablodiegoo/datafolio/internal/toolchain"
)

func TestCheckSlideOverflowVariantsAndErrors(t *testing.T) {
	t.Run("accepts full typst query JSON and field value JSON with zero overflows", func(t *testing.T) {
		for _, payload := range []string{
			`[
				{"func":"metadata","value":{"kind":"start","index":1,"title":"Cover","page":1},"label":"<datafolio-slide>"},
				{"func":"metadata","value":{"kind":"end","index":1,"title":"Cover","page":1},"label":"<datafolio-slide>"}
			]`,
			`[
				{"kind":"start","index":1,"title":"Cover","page":1},
				{"kind":"end","index":1,"title":"Cover","page":1},
				{"kind":"start","index":2,"title":"Method","page":2},
				{"kind":"end","index":2,"title":"Method","page":2}
			]`,
		} {
			runner := &toolchain.FakeTypstRunner{QueryOutput: []byte(payload)}
			got, err := typst.CheckSlideOverflow(context.Background(), runner, toolchain.QueryRequest{InputPath: "slides.typ"})
			if err != nil {
				t.Fatalf("CheckSlideOverflow unexpected error: %v", err)
			}
			if len(got) != 0 {
				t.Fatalf("expected 0 overflows, got %+v", got)
			}
			if len(runner.QueryCalls) != 1 || runner.QueryCalls[0].Selector != typst.SlideMarkerSelector {
				t.Errorf("expected Selector %q, got %+v", typst.SlideMarkerSelector, runner.QueryCalls)
			}
		}
	})

	t.Run("detects multiple overflowing slides and missing boundary markers in sorted order", func(t *testing.T) {
		payload := `[
			{"value":{"kind":"start","index":4,"title":"Summary","page":6}},
			{"value":{"kind":"start","index":2,"title":"Long Proof","page":2}},
			{"value":{"kind":"end","index":2,"title":"Long Proof","page":4}},
			{"value":{"kind":"end","index":3,"title":"Missing Start","page":5}},
			{"value":{"kind":"start","index":1,"title":"Cover","page":1}},
			{"value":{"kind":"end","index":1,"title":"Cover","page":1}}
		]`
		runner := &toolchain.FakeTypstRunner{QueryOutput: []byte(payload)}
		got, err := typst.CheckSlideOverflow(context.Background(), runner, toolchain.QueryRequest{InputPath: "slides.typ"})
		if err != nil {
			t.Fatalf("CheckSlideOverflow unexpected error: %v", err)
		}
		if len(got) != 3 {
			t.Fatalf("got %d overflows, want 3: %+v", len(got), got)
		}
		if got[0].Index != 2 || got[0].Title != "Long Proof" || got[0].StartPage != 2 || got[0].EndPage != 4 || got[0].PageSpan != 3 {
			t.Errorf("unexpected overflow[0]: %+v", got[0])
		}
		if got[1].Index != 3 || got[1].Missing != "start" {
			t.Errorf("unexpected overflow[1]: %+v", got[1])
		}
		if got[2].Index != 4 || got[2].Missing != "end" {
			t.Errorf("unexpected overflow[2]: %+v", got[2])
		}
		msg := typst.FormatSlideOverflows(got)
		if !strings.Contains(msg, `slide 2 ("Long Proof")`) || !strings.Contains(msg, "missing start") || !strings.Contains(msg, "missing end") {
			t.Errorf("unexpected FormatSlideOverflows message: %q", msg)
		}
	})

	t.Run("rejects nil runner, empty output, malformed JSON, and invalid marker data", func(t *testing.T) {
		if _, err := typst.CheckSlideOverflow(context.Background(), nil, toolchain.QueryRequest{}); err == nil {
			t.Errorf("expected error for nil runner")
		}
		for _, bad := range []string{"", "   ", "{not json", `[{"value":{"kind":"bad","index":1,"page":1}}]`, `[{"value":{"kind":"start","index":0,"page":1}}]`} {
			runner := &toolchain.FakeTypstRunner{QueryOutput: []byte(bad)}
			if _, err := typst.CheckSlideOverflow(context.Background(), runner, toolchain.QueryRequest{InputPath: "slides.typ"}); err == nil {
				t.Errorf("expected error for payload %q", bad)
			}
		}
	})
}
