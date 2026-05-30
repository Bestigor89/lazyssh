//go:build embed_lss

package ssh

import _ "embed"

//go:embed embed/lss-linux-amd64
var lssAmd64 []byte

//go:embed embed/lss-linux-arm64
var lssArm64 []byte

func init() {
	lssEmbeds = map[string][]byte{
		"amd64": lssAmd64,
		"arm64": lssArm64,
	}
}
