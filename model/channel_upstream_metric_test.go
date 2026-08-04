package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUpstreamKeyFingerprintCanonicalizesBearerAndSkPrefix(t *testing.T) {
	base := UpstreamKeyFingerprint("token-value")

	require.NotEmpty(t, base)
	require.Equal(t, base, UpstreamKeyFingerprint("Bearer sk-token-value"))
	require.Equal(t, base, UpstreamKeyFingerprint("sk-token-value"))
}

func TestUpstreamKeyFingerprintReturnsEmptyForEmptyKey(t *testing.T) {
	if got := UpstreamKeyFingerprint(" Bearer sk- "); got != "" {
		t.Fatalf("UpstreamKeyFingerprint(empty) = %q, want empty", got)
	}
}
