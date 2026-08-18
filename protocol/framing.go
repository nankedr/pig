package protocol

const DefaultMaxFrameLength uint32 = 16 * 1024 * 1024

// FrameDecoderOptions contains optional frame limits.
type FrameDecoderOptions struct {
	MaxFrameLength *uint32
}

// EncodeFrame is a capability stub until M9.
func EncodeFrame([]byte) ([]byte, error) {
	return nil, newNotImplemented("EncodeFrame")
}

// AssertCompleteFrame is a capability stub until M9.
func AssertCompleteFrame([]byte, *FrameDecoderOptions) error {
	return newNotImplemented("AssertCompleteFrame")
}

// FrameDecoder is a zero-state capability stub.
type FrameDecoder struct{}

// NewFrameDecoder is a capability stub until M9.
func NewFrameDecoder(*FrameDecoderOptions) (*FrameDecoder, error) {
	return nil, newNotImplemented("NewFrameDecoder")
}

// Push is a capability stub until M9.
func (*FrameDecoder) Push([]byte) ([][]byte, error) {
	return nil, newNotImplemented("FrameDecoder.Push")
}

// End is a capability stub until M9.
func (*FrameDecoder) End() error {
	return newNotImplemented("FrameDecoder.End")
}
