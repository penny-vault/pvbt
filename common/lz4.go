package common

import (
	"bytes"
	"io"

	"github.com/pierrec/lz4/v4"
)

func Compress(in []byte) ([]byte, error) {
	r := bytes.NewReader(in)
	w := &bytes.Buffer{}
	zw := lz4.NewWriter(w)
	_, err := io.Copy(zw, r)
	if err != nil {
		return nil, err
	}

	if err := zw.Close(); err != nil {
		return nil, err
	}
	return w.Bytes(), nil
}

func Decompress(in []byte) ([]byte, error) {
	r := bytes.NewReader(in)
	w := &bytes.Buffer{}
	zr := lz4.NewReader(r)
	_, err := io.Copy(w, zr)
	if err != nil {
		return nil, err
	}
	return w.Bytes(), nil
}
