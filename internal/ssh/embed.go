package ssh

// lssEmbeds holds lss binaries indexed by arch ("amd64", "arm64").
// Populated by embed_lss.go when built with -tags embed_lss; empty otherwise.
var lssEmbeds map[string][]byte

// LSSHelper returns the embedded lss binary for the given Linux arch, or nil.
func LSSHelper(arch string) []byte {
	return lssEmbeds[arch]
}
