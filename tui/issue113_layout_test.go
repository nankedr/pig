package tui_test

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/nankedr/pig/tui"
)

type lines113 []string

func (l *lines113) Render(int) ([]string, error) { return append([]string(nil), (*l)...), nil }
func (*lines113) Invalidate() error              { return nil }
func ptr113[T any](v T) *T                       { return &v }
func clean113(lines []string) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		s, _ := tui.StripTerminalSequences(l)
		out[i] = strings.TrimRight(s, " ")
	}
	return out
}

func TestLayoutScrollingOracle113(t *testing.T) {
	var fixture struct {
		Observation struct {
			Outcome struct {
				States []struct {
					Lines     []string
					Top       int
					Following bool
					Viewport  int
					Geometry  tui.ScrollbarGeometry
				}
				Allocation [][]int
				Hits       [][]string
				Horizontal []string
				WideLines  []string
			}
		}
	}
	data, err := os.ReadFile("../parity/oracle/fixtures/layout-scrolling.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	content := lines113{}
	for i := 0; i < 12; i++ {
		content = append(content, fmt.Sprint("row ", i))
	}
	footer := lines113{"input", "cursor" + tui.CursorMarker}
	scroll := tui.NewScrollView(&content, tui.ScrollViewOptions{Follow: ptr113(tui.ScrollViewFollowEnd), Primary: ptr113(true), Scrollbar: ptr113(tui.ScrollViewScrollbarAlways)})
	root := tui.NewVStack([]tui.StackChild{{Entry: &tui.StackEntry{Component: scroll, StackEntryOptions: tui.StackEntryOptions{Basis: &tui.StackBasis{Size: 0}, Grow: ptr113(1)}}}, {Entry: &tui.StackEntry{Component: &footer, StackEntryOptions: tui.StackEntryOptions{Shrink: ptr113(0)}}}})
	for i, want := range fixture.Observation.Outcome.States {
		width, height := 12, 7
		switch i {
		case 1:
			_, err = scroll.ScrollBy(-3)
		case 2:
			content = append(content, "row 12", "row 13")
		case 4:
			err = scroll.ScrollToEnd()
		case 5:
			content = lines113{"short"}
		case 6:
			err = scroll.ScrollToStart()
		}
		if err != nil {
			t.Fatal(err)
		}
		if i >= 3 {
			width, height = 8, 6
		}
		if i == 6 {
			height = 3
		}
		frame, err := tui.RenderLayoutFrame(root, width, height, func() {})
		if err != nil {
			t.Fatal(err)
		}
		box, ok, err := tui.GetScrollViewBox(frame, scroll)
		if err != nil || !ok {
			t.Fatalf("box %v %v", ok, err)
		}
		geometry, ok, err := tui.GetScrollbarGeometry(box)
		if err != nil || !ok {
			t.Fatalf("geometry %v %v", ok, err)
		}
		if !reflect.DeepEqual(clean113(frame.Lines), want.Lines) || scroll.ScrollTop() != want.Top || scroll.IsFollowingEnd() != want.Following || scroll.ViewportHeight() != want.Viewport || geometry != want.Geometry {
			t.Fatalf("step %d: lines=%q top=%d follow=%v viewport=%d geometry=%+v; want %+v", i, clean113(frame.Lines), scroll.ScrollTop(), scroll.IsFollowingEnd(), scroll.ViewportHeight(), geometry, want)
		}
	}
	wide := lines113{"中文", "中文", "中文", "中文"}
	view := tui.NewScrollView(&wide, tui.ScrollViewOptions{Scrollbar: ptr113(tui.ScrollViewScrollbarAlways)})
	frame, err := tui.RenderLayoutFrame(view, 3, 2, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(clean113(frame.Lines), fixture.Observation.Outcome.WideLines) {
		t.Fatalf("wide clipped frame %q", frame.Lines)
	}

}

func TestLayoutAllocationAndNestedHits113(t *testing.T) {
	var fixture struct {
		Case struct {
			Input struct {
				Allocation []struct {
					Entries        []struct{ Basis, Grow, Shrink, MinSize, MaxSize *int }
					Intrinsic      []int
					Available, Gap int
				}
			}
		}
		Observation struct {
			Outcome struct {
				Allocation [][]int
				Hits       [][]string
				Horizontal []string
			}
		}
	}
	data, err := os.ReadFile("../parity/oracle/fixtures/layout-scrolling.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	for i, c := range fixture.Case.Input.Allocation {
		entries := make([]tui.StackEntry, len(c.Entries))
		for j, e := range c.Entries {
			entries[j].StackEntryOptions = tui.StackEntryOptions{Grow: e.Grow, Shrink: e.Shrink, MinSize: e.MinSize, MaxSize: e.MaxSize}
			if e.Basis != nil {
				entries[j].Basis = &tui.StackBasis{Size: *e.Basis}
			}
		}
		got, err := tui.AllocateStackSizes(entries, c.Intrinsic, &c.Available, c.Gap)
		if err != nil || !reflect.DeepEqual(got, fixture.Observation.Outcome.Allocation[i]) {
			t.Fatalf("allocation %d: %v %v", i, got, err)
		}
	}
	content := lines113{"a", "b", "c", "d", "e"}
	footer := lines113{"input", "cursor" + tui.CursorMarker}
	inner := tui.NewScrollView(&content, tui.ScrollViewOptions{Scrollbar: ptr113(tui.ScrollViewScrollbarAlways)})
	outer := tui.NewScrollView(tui.NewVStack([]tui.StackChild{{Entry: &tui.StackEntry{Component: inner, StackEntryOptions: tui.StackEntryOptions{Basis: &tui.StackBasis{Size: 3}}}}, {Entry: &tui.StackEntry{Component: &footer, StackEntryOptions: tui.StackEntryOptions{Basis: &tui.StackBasis{Size: 6}}}}}), tui.ScrollViewOptions{Primary: ptr113(true)})
	frame, err := tui.RenderLayoutFrame(outer, 12, 5, nil)
	if err != nil {
		t.Fatal(err)
	}
	for i, point := range [][2]int{{1, 1}, {1, 4}, {12, 0}} {
		hits, err := tui.GetScrollViewsAt(frame, point[0], point[1])
		if err != nil {
			t.Fatal(err)
		}
		names := []string{}
		for _, s := range hits {
			if s == inner {
				names = append(names, "inner")
			} else if s == outer {
				names = append(names, "outer")
			} else {
				t.Fatal("unexpected scroll target")
			}
		}
		if !reflect.DeepEqual(names, fixture.Observation.Outcome.Hits[i]) {
			t.Fatalf("hits %v", names)
		}
	}
	short := lines113{"short"}
	horizontal := tui.NewHStack([]tui.StackChild{{Entry: &tui.StackEntry{Component: &footer, StackEntryOptions: tui.StackEntryOptions{Basis: &tui.StackBasis{Size: 5}}}}, {Entry: &tui.StackEntry{Component: &short, StackEntryOptions: tui.StackEntryOptions{Grow: ptr113(1)}}}}, tui.StackOptions{Gap: ptr113(1), Align: ptr113(tui.StackAlignEnd)})
	frame, err = tui.RenderLayoutFrame(horizontal, 12, 4, nil)
	if err != nil || !reflect.DeepEqual(clean113(frame.Lines), fixture.Observation.Outcome.Horizontal) {
		t.Fatalf("horizontal %q %v", clean113(frame.Lines), err)
	}
}
