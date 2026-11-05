// SPDX-License-Identifier: MIT

//go:build unix

package fsutil

import "syscall"

// oNoFollow makes opening an output fail on a symbolic link.
const oNoFollow = syscall.O_NOFOLLOW
