package protocol

// ParseClientMessage is a capability stub until M9.
func ParseClientMessage(any) (ClientMessage, error) {
	return nil, newNotImplemented("ParseClientMessage")
}

// ParseServerMessage is a capability stub until M9.
func ParseServerMessage(any) (ServerMessage, error) {
	return nil, newNotImplemented("ParseServerMessage")
}

// EncodeClientMessage is a capability stub until M9.
func EncodeClientMessage(ClientMessage, *FrameDecoderOptions) ([]byte, error) {
	return nil, newNotImplemented("EncodeClientMessage")
}

// EncodeServerMessage is a capability stub until M9.
func EncodeServerMessage(ServerMessage, *FrameDecoderOptions) ([]byte, error) {
	return nil, newNotImplemented("EncodeServerMessage")
}

// ClientMessageDecoder is a zero-state capability stub.
type ClientMessageDecoder struct{}

// NewClientMessageDecoder is a capability stub until M9.
func NewClientMessageDecoder(*FrameDecoderOptions) (*ClientMessageDecoder, error) {
	return nil, newNotImplemented("NewClientMessageDecoder")
}

// Push is a capability stub until M9.
func (*ClientMessageDecoder) Push([]byte) ([]ClientMessage, error) {
	return nil, newNotImplemented("ClientMessageDecoder.Push")
}

// End is a capability stub until M9.
func (*ClientMessageDecoder) End() error {
	return newNotImplemented("ClientMessageDecoder.End")
}

// ServerMessageDecoder is a zero-state capability stub.
type ServerMessageDecoder struct{}

// NewServerMessageDecoder is a capability stub until M9.
func NewServerMessageDecoder(*FrameDecoderOptions) (*ServerMessageDecoder, error) {
	return nil, newNotImplemented("NewServerMessageDecoder")
}

// Push is a capability stub until M9.
func (*ServerMessageDecoder) Push([]byte) ([]ServerMessage, error) {
	return nil, newNotImplemented("ServerMessageDecoder.Push")
}

// End is a capability stub until M9.
func (*ServerMessageDecoder) End() error {
	return newNotImplemented("ServerMessageDecoder.End")
}
