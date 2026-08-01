package models

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// NewID generates a prefixed random ID (e.g. "srv_a1b2c3d4e5f6").
func NewID(prefix string) string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%s_%s", prefix, hex.EncodeToString(b))
}

// Now returns the current time, centralized for testability.
func Now() time.Time { return time.Now() }
