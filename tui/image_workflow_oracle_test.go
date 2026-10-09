package tui_test

import (
	"encoding/json"
	"github.com/nankedr/pig/tui"
	"os"
	"testing"
)

func TestImageWorkflowKittyOracle136(t *testing.T) {
	data, err := os.ReadFile("../parity/oracle/fixtures/image-workflow.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		BaselineCommit string `json:"baseline_commit"`
		Cases          []struct {
			Data, Sequence, Deletion string
			Options                  struct {
				Columns, Rows int
				ImageID       uint32
				MoveCursor    *bool
			}
		}
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.BaselineCommit != "936aff00918de1187f085f123c2812d8f2d67745" || len(fixture.Cases) != 2 {
		t.Fatal("invalid fixed Oracle")
	}
	for _, tc := range fixture.Cases {
		sequence, err := tui.EncodeKitty(tc.Data, tui.KittyEncodeOptions{Columns: &tc.Options.Columns, Rows: &tc.Options.Rows, ImageID: &tc.Options.ImageID, MoveCursor: tc.Options.MoveCursor})
		if err != nil || sequence != tc.Sequence {
			t.Fatal("Kitty chunk/placement differs from Pi", err)
		}
		deletion, err := tui.DeleteKittyImage(tc.Options.ImageID)
		if err != nil || deletion != tc.Deletion {
			t.Fatal("Kitty deletion differs from Pi", err)
		}
	}
	for _, bad := range []string{"AAAA\x1b_Gattack", "AAAA\n", "data:image/png;base64,AAAA"} {
		if _, err := tui.EncodeKitty(bad); err == nil {
			t.Fatal("unsafe terminal payload accepted")
		}
	}
}
