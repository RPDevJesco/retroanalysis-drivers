package drivers

import (
	"fmt"
	"github.com/RPDevJesco/retroanalysis-drivers/memory"
	"net"
	"strconv"
	"strings"
	"time"
)

// AdaptiveRetroArchDriver automatically chunks large reads while maintaining performance
type AdaptiveRetroArchDriver struct {
	host           string
	port           int
	requestTimeout time.Duration
	conn           *net.UDPConn

	// Adaptive chunking parameters
	maxChunkSize  uint32 // Maximum bytes to read in one request
	bufferSize    int    // Socket buffer size
	testChunkSize uint32 // Size to test for optimal chunk size
}

// Platform-specific configurations
var platformConfigs = map[string]struct {
	maxChunkSize uint32
	bufferSize   int
}{
	// Game Boy variants
	"GB":       {maxChunkSize: 1024, bufferSize: 64 * 1024},
	"GAME BOY": {maxChunkSize: 1024, bufferSize: 64 * 1024},
	"GBC":      {maxChunkSize: 1024, bufferSize: 64 * 1024},
	"GAMEBOY":  {maxChunkSize: 1024, bufferSize: 64 * 1024},

	// Game Boy Advance variants
	"GBA":              {maxChunkSize: 2048, bufferSize: 256 * 1024},
	"GAME BOY ADVANCE": {maxChunkSize: 2048, bufferSize: 256 * 1024},

	// Nintendo variants
	"NES":            {maxChunkSize: 1024, bufferSize: 64 * 1024},
	"NINTENDO":       {maxChunkSize: 1024, bufferSize: 64 * 1024},
	"SNES":           {maxChunkSize: 2048, bufferSize: 128 * 1024},
	"SUPER NINTENDO": {maxChunkSize: 2048, bufferSize: 128 * 1024},

	// Nintendo DS variants
	"NDS":          {maxChunkSize: 4096, bufferSize: 2 * 1024 * 1024},
	"NINTENDO DS":  {maxChunkSize: 4096, bufferSize: 2 * 1024 * 1024},
	"DSI":          {maxChunkSize: 8192, bufferSize: 4 * 1024 * 1024},
	"NINTENDO DSI": {maxChunkSize: 8192, bufferSize: 4 * 1024 * 1024},
}

// NewAdaptiveRetroArchDriver creates a new adaptive RetroArch driver
func NewAdaptiveRetroArchDriver(host string, port int, requestTimeout time.Duration) *AdaptiveRetroArchDriver {
	return &AdaptiveRetroArchDriver{
		host:           host,
		port:           port,
		requestTimeout: requestTimeout,
		maxChunkSize:   2048,        // Default safe chunk size
		bufferSize:     1024 * 1024, // Default 1MB buffer
		testChunkSize:  1024,        // Start testing with 1KB
	}
}

// SetPlatform configures optimal settings for a specific platform
func (d *AdaptiveRetroArchDriver) SetPlatform(platform string) {
	// Normalize platform name (uppercase, trim spaces)
	normalizedPlatform := strings.ToUpper(strings.TrimSpace(platform))

	if config, exists := platformConfigs[normalizedPlatform]; exists {
		d.maxChunkSize = config.maxChunkSize
		d.bufferSize = config.bufferSize
		fmt.Printf("🎮 Configured for %s: chunk_size=%d, buffer_size=%d\n",
			platform, d.maxChunkSize, d.bufferSize)
	} else {
		fmt.Printf("⚠️  Unknown platform '%s', using defaults (chunk_size=%d, buffer_size=%d)\n",
			platform, d.maxChunkSize, d.bufferSize)
	}
}

// Connect establishes connection to RetroArch and auto-detects optimal settings
func (d *AdaptiveRetroArchDriver) Connect() error {
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", d.host, d.port))
	if err != nil {
		return fmt.Errorf("failed to resolve UDP address: %w", err)
	}

	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		return fmt.Errorf("failed to connect to RetroArch: %w", err)
	}

	// Set buffer sizes based on platform
	if err := conn.SetReadBuffer(d.bufferSize); err != nil {
		fmt.Printf("⚠️  Warning: failed to set read buffer to %d: %v\n", d.bufferSize, err)
	}

	if err := conn.SetWriteBuffer(d.bufferSize); err != nil {
		fmt.Printf("⚠️  Warning: failed to set write buffer to %d: %v\n", d.bufferSize, err)
	}

	d.conn = conn

	// Test connection and auto-detect optimal chunk size
	if err := d.testConnection(); err != nil {
		d.conn.Close()
		d.conn = nil
		return fmt.Errorf("failed to communicate with RetroArch: %w", err)
	}

	return nil
}

// testConnection tests the connection and finds optimal chunk size
func (d *AdaptiveRetroArchDriver) testConnection() error {
	// First, test basic connectivity
	_, err := d.sendCommand("VERSION")
	if err != nil {
		return fmt.Errorf("basic connectivity test failed: %w", err)
	}

	// Try to auto-detect optimal chunk size by testing progressively larger reads
	fmt.Print("🔍 Auto-detecting optimal chunk size... ")

	testSizes := []uint32{512, 1024, 2048, 4096, 8192, 16384}
	optimalSize := uint32(512)    // Fallback to very safe size
	platformMax := d.maxChunkSize // Remember platform-configured maximum

	for _, size := range testSizes {
		// Don't test sizes larger than platform maximum unless platform max is very small
		if size > platformMax && platformMax >= 1024 {
			break
		}

		// Test reading a small chunk at address 0
		command := fmt.Sprintf("READ_CORE_MEMORY 0 %d", size)
		_, err := d.sendCommand(command)

		if err != nil {
			// If this size fails, stop and use the previous working size
			break
		}

		optimalSize = size
	}

	// Use the smaller of: detected optimal size or platform maximum
	if optimalSize > platformMax && platformMax >= 512 {
		fmt.Printf("optimal chunk size: %d bytes (limited by platform config: %d)\n", platformMax, optimalSize)
		d.maxChunkSize = platformMax
	} else {
		fmt.Printf("optimal chunk size: %d bytes\n", optimalSize)
		d.maxChunkSize = optimalSize
	}

	return nil
}

// ReadMemoryBlocks reads multiple memory blocks from RetroArch
func (d *AdaptiveRetroArchDriver) ReadMemoryBlocks(blocks []memory.Block) (map[uint32][]byte, error) {
	if d.conn == nil {
		if err := d.Connect(); err != nil {
			return nil, err
		}
	}

	result := make(map[uint32][]byte)

	for _, block := range blocks {
		data, err := d.ReadMemory(block.Start, block.End-block.Start+1)
		if err != nil {
			return nil, fmt.Errorf("failed to read block %s: %w", block.Name, err)
		}
		result[block.Start] = data
	}

	return result, nil
}

// ReadMemory reads memory using adaptive chunking
func (d *AdaptiveRetroArchDriver) ReadMemory(address uint32, length uint32) ([]byte, error) {
	if d.conn == nil {
		return nil, fmt.Errorf("not connected to RetroArch")
	}

	// For small reads, try single request first
	if length <= d.maxChunkSize {
		data, err := d.readChunk(address, length)
		if err == nil {
			return data, nil
		}
		// If single request fails, fall back to chunking
		fmt.Printf("⚠️  Single read failed for %d bytes, falling back to chunking\n", length)
	}

	// Use chunked reading for large requests or when single read fails
	return d.readChunked(address, length)
}

// readChunked reads memory in chunks
func (d *AdaptiveRetroArchDriver) readChunked(address uint32, length uint32) ([]byte, error) {
	result := make([]byte, 0, length)
	remaining := length
	currentAddr := address

	for remaining > 0 {
		chunkSize := d.maxChunkSize
		if remaining < chunkSize {
			chunkSize = remaining
		}

		chunk, err := d.readChunk(currentAddr, chunkSize)
		if err != nil {
			return nil, fmt.Errorf("failed to read chunk at 0x%X: %w", currentAddr, err)
		}

		result = append(result, chunk...)
		currentAddr += chunkSize
		remaining -= chunkSize
	}

	return result, nil
}

// readChunk reads a single chunk of memory
func (d *AdaptiveRetroArchDriver) readChunk(address uint32, length uint32) ([]byte, error) {
	// Convert address to hex string (RetroArch expects lowercase hex without 0x prefix)
	addrStr := fmt.Sprintf("%x", address)

	// Build command: READ_CORE_MEMORY <address> <length>
	command := fmt.Sprintf("READ_CORE_MEMORY %s %d", addrStr, length)

	response, err := d.sendCommand(command)
	if err != nil {
		return nil, err
	}

	// Parse response: "READ_CORE_MEMORY <address> <byte1> <byte2> ..."
	parts := strings.Fields(response)
	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid response format: %s", response)
	}

	// Check for error response
	if parts[2] == "-1" {
		return nil, fmt.Errorf("RetroArch returned error for address %s", addrStr)
	}

	// Skip command name and address, parse the bytes
	byteStrings := parts[2:]
	if len(byteStrings) != int(length) {
		return nil, fmt.Errorf("expected %d bytes, got %d", length, len(byteStrings))
	}

	data := make([]byte, length)
	for i, byteStr := range byteStrings {
		b, err := strconv.ParseUint(byteStr, 16, 8)
		if err != nil {
			return nil, fmt.Errorf("invalid byte value %s: %w", byteStr, err)
		}
		data[i] = byte(b)
	}

	return data, nil
}

// WriteBytes writes bytes to RetroArch
func (d *AdaptiveRetroArchDriver) WriteBytes(address uint32, data []byte) error {
	if d.conn == nil {
		return fmt.Errorf("not connected to RetroArch")
	}

	// For large writes, chunk them too
	if len(data) > int(d.maxChunkSize) {
		return d.writeChunked(address, data)
	}

	return d.writeChunk(address, data)
}

// writeChunked writes data in chunks
func (d *AdaptiveRetroArchDriver) writeChunked(address uint32, data []byte) error {
	remaining := len(data)
	currentAddr := address
	offset := 0

	for remaining > 0 {
		chunkSize := int(d.maxChunkSize)
		if remaining < chunkSize {
			chunkSize = remaining
		}

		chunk := data[offset : offset+chunkSize]
		if err := d.writeChunk(currentAddr, chunk); err != nil {
			return fmt.Errorf("failed to write chunk at 0x%X: %w", currentAddr, err)
		}

		currentAddr += uint32(chunkSize)
		remaining -= chunkSize
		offset += chunkSize
	}

	return nil
}

// writeChunk writes a single chunk
func (d *AdaptiveRetroArchDriver) writeChunk(address uint32, data []byte) error {
	// Convert bytes to hex strings
	hexBytes := make([]string, len(data))
	for i, b := range data {
		hexBytes[i] = fmt.Sprintf("%02x", b)
	}

	// Build command: WRITE_CORE_MEMORY <address> <byte1> <byte2> ...
	addrStr := fmt.Sprintf("%x", address)
	command := fmt.Sprintf("WRITE_CORE_MEMORY %s %s", addrStr, strings.Join(hexBytes, " "))

	_, err := d.sendCommand(command)
	return err
}

// Close closes the connection
func (d *AdaptiveRetroArchDriver) Close() error {
	if d.conn != nil {
		err := d.conn.Close()
		d.conn = nil
		return err
	}
	return nil
}

// sendCommand sends a command to RetroArch and returns the response
func (d *AdaptiveRetroArchDriver) sendCommand(command string) (string, error) {
	if d.conn == nil {
		return "", fmt.Errorf("not connected")
	}

	// Set timeout for this operation
	if err := d.conn.SetDeadline(time.Now().Add(d.requestTimeout)); err != nil {
		return "", fmt.Errorf("failed to set deadline: %w", err)
	}

	// Send command
	_, err := d.conn.Write([]byte(command))
	if err != nil {
		return "", fmt.Errorf("failed to send command: %w", err)
	}

	// Read response with platform-appropriate buffer
	bufferSize := 32 * 1024 // 32KB default for responses
	if d.maxChunkSize > 1024 {
		bufferSize = int(d.maxChunkSize) * 4 // Scale buffer with chunk size
	}

	buffer := make([]byte, bufferSize)
	n, err := d.conn.Read(buffer)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	response := strings.TrimSpace(string(buffer[:n]))
	return response, nil
}
