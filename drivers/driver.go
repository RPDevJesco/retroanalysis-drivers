package drivers

import (
	"github.com/RPDevJesco/retroanalysis-drivers/memory"
)

// Driver interface for emulator communication
type Driver interface {
	// Connect establishes connection to the emulator
	Connect() error

	// ReadMemoryBlocks reads multiple memory blocks
	ReadMemoryBlocks(blocks []memory.Block)

	// WriteBytes writes data to a specific memory address
	WriteBytes(address uint32, data []byte) error

	// Close closes the connection
	Close() error
}
