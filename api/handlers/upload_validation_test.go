package handlers

import "testing"

func TestValidateUploadMagic(t *testing.T) {
	cases := []struct {
		name string
		kind uploadKind
		head []byte
		ok   bool
	}{
		{"pdf", uploadPDF, []byte("%PDF-1.7\n"), true},
		{"fake pdf", uploadPDF, []byte("hello"), false},
		{"png", uploadImage, []byte("\x89PNG\r\n\x1a\n"), true},
		{"fake png", uploadImage, []byte("hello"), false},
		{"text", uploadText, []byte("hello world"), true},
		{"zip", uploadZip, []byte("PK\x03\x04"), true},
		{"fake zip", uploadZip, []byte("hello"), false},
	}
	for _, c := range cases {
		err := validateUploadMagic(c.kind, c.head)
		if c.ok && err != nil {
			t.Fatalf("%s should pass: %v", c.name, err)
		}
		if !c.ok && err == nil {
			t.Fatalf("%s should fail", c.name)
		}
	}
}
