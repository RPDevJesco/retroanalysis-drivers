package drivers

import (
	"go/types"
)

// Driver interface for emulator communication
type Driver interface {
	// Connect establishes connection to the emulator
	Connect() error

	// ReadMemoryBlocks reads multiple memory blocks
	ReadMemoryBlocks(blocks []types.MemoryBlock) (map[uint32][]byte, error)

	// WriteBytes writes data to a specific memory address
	WriteBytes(address uint32, data []byte) error

	// Close closes the connection
	Close() error
}
