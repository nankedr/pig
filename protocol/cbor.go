package protocol

const (
	DefaultMaxCBORByteLength      uint32 = 16 * 1024 * 1024
	DefaultMaxCBORContainerLength uint32 = 1_000_000
	DefaultMaxCBORDepth           uint32 = 64
)

// CBOROptions contains optional safety limits. Pointer fields preserve the
// distinction between absence and an explicit zero limit.
type CBOROptions struct {
	MaxByteLength      *uint32
	MaxContainerLength *uint32
	MaxDepth           *uint32
}

// EncodeCBOR is a capability stub until M9.
func EncodeCBOR(any, *CBOROptions) ([]byte, error) {
	return nil, newNotImplemented("EncodeCBOR")
}

// DecodeCBOR is a capability stub until M9.
func DecodeCBOR([]byte, *CBOROptions) (any, error) {
	return nil, newNotImplemented("DecodeCBOR")
}
