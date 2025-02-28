package airdrop

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGetNextAirdropTime(t *testing.T) {
	inauguralTs := time.Now().Add(-24 * time.Hour)
	nextAirdrop := getNextAirdropTime(inauguralTs)
	require.InDelta(t, 6*24*time.Hour, time.Until(nextAirdrop), float64(time.Second))

	inauguralTs = time.Now().Add(time.Hour)
	nextAirdrop = getNextAirdropTime(inauguralTs)
	require.Equal(t, inauguralTs, nextAirdrop)
}
