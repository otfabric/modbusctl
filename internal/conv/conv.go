// SPDX-License-Identifier: MIT

// Package conv converts lengths, counts and addresses into the fixed-width
// fields of Modbus frames. The callers bound the value first (a protocol
// limit or the frame size), so a value that does not fit is a bug in
// modbusctl: these functions panic rather than let a field wrap.
package conv

import (
	"fmt"
	"math"
)

// Uint8 returns n for a one-byte count or length field.
func Uint8(n int) uint8 {
	if n < 0 || n > math.MaxUint8 {
		panic(fmt.Sprintf("modbusctl: %d does not fit in a byte field", n))
	}
	return uint8(n)
}

// Uint16 returns n for a two-byte count, length or address field.
func Uint16(n int) uint16 {
	if n < 0 || n > math.MaxUint16 {
		panic(fmt.Sprintf("modbusctl: %d does not fit in a 16-bit field", n))
	}
	return uint16(n)
}

// Addr16 returns the Modbus address a for a 32-bit address computation.
func Addr16(a uint32) uint16 {
	if a > math.MaxUint16 {
		panic(fmt.Sprintf("modbusctl: address %d is beyond 65535", a))
	}
	return uint16(a)
}
