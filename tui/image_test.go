package tui_test

import (
	"errors"
	"testing"

	"github.com/nankedr/pig/tui"
)

func TestImageAndTextHelpersAreExplicitCapabilityStubs(t *testing.T) {
	probeCalls := 0
	backgroundCalls := 0
	textCalls := 0
	maxHeight := 20
	dimensions := tui.ImageDimensions{WidthPX: 640, HeightPX: 480}

	operations := []struct {
		name string
		call func() error
	}{
		{name: "getCellDimensions", call: func() error { _, err := tui.GetCellDimensions(); return err }},
		{name: "setCellDimensions", call: func() error { return tui.SetCellDimensions(tui.CellDimensions{WidthPX: 9, HeightPX: 18}) }},
		{name: "detectCapabilities", call: func() error {
			_, err := tui.DetectCapabilities(func() bool { probeCalls++; return true })
			return err
		}},
		{name: "getCapabilities", call: func() error { _, err := tui.GetCapabilities(); return err }},
		{name: "resetCapabilitiesCache", call: tui.ResetCapabilitiesCache},
		{name: "setCapabilities", call: func() error {
			return tui.SetCapabilities(tui.TerminalCapabilities{Images: tui.ImageProtocolKitty, TrueColor: true, Hyperlinks: true})
		}},
		{name: "isImageLine", call: func() error { _, err := tui.IsImageLine("plain text"); return err }},
		{name: "allocateImageId", call: func() error { _, err := tui.AllocateImageID(); return err }},
		{name: "encodeKitty", call: func() error { _, err := tui.EncodeKitty("AAAA"); return err }},
		{name: "deleteKittyImage", call: func() error { _, err := tui.DeleteKittyImage(1); return err }},
		{name: "deleteAllKittyImages", call: func() error { _, err := tui.DeleteAllKittyImages(); return err }},
		{name: "deleteAllKittyPlacements", call: func() error { _, err := tui.DeleteAllKittyPlacements(); return err }},
		{name: "encodeITerm2", call: func() error { _, err := tui.EncodeITerm2("AAAA"); return err }},
		{name: "registerKittyImageMetadata", call: func() error {
			return tui.RegisterKittyImageMetadata(tui.KittyImageMetadata{ImageID: 1})
		}},
		{name: "getKittyImageMetadata", call: func() error { _, _, err := tui.GetKittyImageMetadata("line"); return err }},
		{name: "getKittyImagePlacement", call: func() error { _, _, err := tui.GetKittyImagePlacement("line"); return err }},
		{name: "cropKittyImageLine", call: func() error { _, err := tui.CropKittyImageLine("line", 1, 2); return err }},
		{name: "calculateImageCellSize", call: func() error {
			_, err := tui.CalculateImageCellSize(dimensions, 80, &maxHeight)
			return err
		}},
		{name: "calculateImageRows", call: func() error { _, err := tui.CalculateImageRows(dimensions, 80); return err }},
		{name: "getPngDimensions", call: func() error { _, _, err := tui.GetPNGDimensions("AAAA"); return err }},
		{name: "getJpegDimensions", call: func() error { _, _, err := tui.GetJPEGDimensions("AAAA"); return err }},
		{name: "getGifDimensions", call: func() error { _, _, err := tui.GetGIFDimensions("AAAA"); return err }},
		{name: "getWebpDimensions", call: func() error { _, _, err := tui.GetWebPDimensions("AAAA"); return err }},
		{name: "getImageDimensions", call: func() error { _, _, err := tui.GetImageDimensions("AAAA", "image/png"); return err }},
		{name: "renderImage", call: func() error { _, _, err := tui.RenderImage("AAAA", dimensions); return err }},
		{name: "hyperlink", call: func() error { _, err := tui.Hyperlink("text", "https://example.com"); return err }},
		{name: "imageFallback", call: func() error { _, err := tui.ImageFallback("image/png", &dimensions, "/image.png"); return err }},
		{name: "getGraphemeSegmenter", call: func() error { _, err := tui.GetGraphemeSegmenter(); return err }},
		{name: "getWordSegmenter", call: func() error { _, err := tui.GetWordSegmenter(); return err }},
		{name: "visibleWidth", call: func() error { _, err := tui.VisibleWidth("text"); return err }},
		{name: "stripTerminalSequences", call: func() error { _, err := tui.StripTerminalSequences("text"); return err }},
		{name: "getGraphemeCellRange", call: func() error { _, _, err := tui.GetGraphemeCellRange("text", 0); return err }},
		{name: "getOsc8LinkAtColumn", call: func() error { _, _, err := tui.GetOSC8LinkAtColumn("text", 0); return err }},
		{name: "normalizeTerminalOutput", call: func() error { _, err := tui.NormalizeTerminalOutput("text"); return err }},
		{name: "extractAnsiCode", call: func() error { _, _, err := tui.ExtractANSICode("text", 0); return err }},
		{name: "wrapTextWithAnsi", call: func() error { _, err := tui.WrapTextWithANSI("text", 10); return err }},
		{name: "isWhitespaceChar", call: func() error { _, err := tui.IsWhitespaceChar(" "); return err }},
		{name: "isPunctuationChar", call: func() error { _, err := tui.IsPunctuationChar("!"); return err }},
		{name: "applyBackgroundToLine", call: func() error {
			_, err := tui.ApplyBackgroundToLine("text", 10, func(value string) string { backgroundCalls++; return value })
			return err
		}},
		{name: "truncateToWidth", call: func() error { _, err := tui.TruncateToWidth("text", 2); return err }},
		{name: "sliceByColumn", call: func() error { _, err := tui.SliceByColumn("text", 0, 2); return err }},
		{name: "sliceWithWidth", call: func() error { _, err := tui.SliceWithWidth("text", 0, 2); return err }},
		{name: "extractSegments", call: func() error { _, err := tui.ExtractSegments("text", 1, 2, 2); return err }},
		{name: "fuzzyMatch", call: func() error { _, err := tui.FuzzyMatch("tx", "text"); return err }},
		{name: "fuzzyFilter", call: func() error {
			_, err := tui.FuzzyFilter([]string{"text"}, "tx", func(value string) string { textCalls++; return value })
			return err
		}},
		{name: "renderLatex", call: func() error { _, _, err := tui.RenderLaTeX(`\alpha`); return err }},
	}

	for _, operation := range operations {
		operation := operation
		t.Run(operation.name, func(t *testing.T) {
			err := operation.call()
			if !errors.Is(err, tui.ErrNotImplemented) {
				t.Fatalf("errors.Is(%v, ErrNotImplemented) = false", err)
			}
			var capabilityErr *tui.NotImplementedError
			if !errors.As(err, &capabilityErr) {
				t.Fatalf("errors.As(%v, *NotImplementedError) = false", err)
			}
			if capabilityErr.Module != "tui" || capabilityErr.Operation != operation.name {
				t.Fatalf("NotImplementedError = %+v, want module tui and operation %s", capabilityErr, operation.name)
			}
		})
	}

	if probeCalls != 0 || backgroundCalls != 0 || textCalls != 0 {
		t.Fatalf("stub callback side effects: probe=%d background=%d text=%d", probeCalls, backgroundCalls, textCalls)
	}
}
