package main

import "io"

const (
	fData   byte = 0
	fWinch  byte = 1
	fDetach byte = 2
)

// Each frame: [type:1][lenHi:1][lenMid:1][lenLo:1][payload...]
// Max payload: 16 MB (more than enough for terminal I/O).

func writeFrame(w io.Writer, typ byte, payload []byte) error {
	n := len(payload)
	hdr := [4]byte{typ, byte(n >> 16), byte(n >> 8), byte(n)}
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	if n > 0 {
		_, err := w.Write(payload)
		return err
	}
	return nil
}

func readFrame(r io.Reader) (byte, []byte, error) {
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return 0, nil, err
	}
	typ := hdr[0]
	length := uint32(hdr[1])<<16 | uint32(hdr[2])<<8 | uint32(hdr[3])
	if length == 0 {
		return typ, nil, nil
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(r, payload); err != nil {
		return 0, nil, err
	}
	return typ, payload, nil
}
