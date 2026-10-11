package handler

import (
	"bytes"
	"encoding/json"
	"os"
)

type jsonListing[T any] struct {
	Data []T `json:"data"`
}

// encodeListing never yields "data": null, because scripts iterate over it.
func encodeListing[T any](entries []T) ([]byte, error) {
	if entries == nil {
		entries = []T{}
	}
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(jsonListing[T]{Data: entries}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// printListing takes a finished document so a failure never leaves half of
// one on stdout.
func printListing(doc []byte) error {
	_, err := os.Stdout.Write(doc)
	return err
}
