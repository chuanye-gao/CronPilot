package id

import (
	"crypto/rand"
	"encoding/hex"
	"strconv"
	"sync/atomic"
	"time"
)

var fallbackCounter atomic.Uint64

func New(prefix string) string {
	var random [8]byte
	if _, err := rand.Read(random[:]); err == nil {
		return prefix + "_" + hex.EncodeToString(random[:])
	}
	fallback := uint64(time.Now().UnixNano()) + fallbackCounter.Add(1)
	return prefix + "_" + strconv.FormatUint(fallback, 16)
}
