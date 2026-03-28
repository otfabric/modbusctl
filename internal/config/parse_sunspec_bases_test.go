package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseSunSpecBases_dedupesPreservesOrder(t *testing.T) {
	got, err := ParseSunSpecBases("40000, 40000, 50000 , 40000")
	assert.NoError(t, err)
	assert.Equal(t, []uint16{40000, 50000}, got)
}
