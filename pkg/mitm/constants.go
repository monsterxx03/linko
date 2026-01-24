package mitm

const (
	// DefaultMaxBodySize 默认最大请求/响应体大小 (16KB)
	DefaultMaxBodySize = 16 * 1024

	// DefaultBufferSize 默认缓冲区大小 (16KB)
	DefaultBufferSize = 16 * 1024

	// CopyBufferSize 用于io.CopyBuffer的缓冲区大小 (256KB)
	CopyBufferSize = 256 * 1024
)
