package protocol

import (
	"encoding/base64"
	"strings"
)

// Base64Decode tries standard and urlsafe base64, with optional padding fix.
func Base64Decode(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", false
	}
	// 补齐 padding
	if pad := len(s) % 4; pad != 0 {
		s += strings.Repeat("=", 4-pad)
	}
	// 尝试 urlsafe
	if b, err := base64.URLEncoding.DecodeString(s); err == nil {
		return string(b), true
	}
	if b, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(s, "=")); err == nil {
		return string(b), true
	}
	// 尝试标准
	if b, err := base64.StdEncoding.DecodeString(s); err == nil {
		return string(b), true
	}
	if b, err := base64.RawStdEncoding.DecodeString(strings.TrimRight(s, "=")); err == nil {
		return string(b), true
	}
	return "", false
}

// IsBase64ish reports whether s looks like base64 (charset + length%4).
func IsBase64ish(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" || len(s)%4 != 0 {
		return false
	}
	for _, c := range s {
		if !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') ||
			(c >= '0' && c <= '9') || c == '+' || c == '/' || c == '=' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}
