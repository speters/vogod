package optolink

import (
	"io"
)

const (
    NUL byte = 0x00
	SOH   byte = 0x01 // Start of heading - used for start of KW frame
    EOT byte = 0x04 // End of transmission - also used similar to a reset from P300 to KW
	ENQ   byte = 0x05 // "ping" in KW mode
    ACK byte = 0x06 // Acknowledge in P300
    SYN byte = 0x16 // Start of sync sequence SYN NUL NULL in P300, switches also from KW to P300
    SO3 byte = 0x41 // Start of frame in P300, ASCII "a"
)

const P300SYN  = []byte

type ReadWriteCloser interface {
	Reader
	Writer
	io.Closer
}
