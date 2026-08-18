package protocol

// ProtocolVersion is the only Remote Session Protocol version supported by
// this compatibility surface.
const ProtocolVersion = 1

// IsSupportedProtocolVersion reports whether version is supported by this
// compatibility surface.
func IsSupportedProtocolVersion(version int) bool {
	return version == ProtocolVersion
}
