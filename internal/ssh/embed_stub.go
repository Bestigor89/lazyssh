//go:build !embed_lss

package ssh

func init() {
	lssEmbeds = map[string][]byte{}
}
