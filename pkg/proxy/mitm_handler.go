package proxy

import (
	"fmt"
	"log/slog"
	"net"

	"github.com/monsterxx03/linko/pkg/mitm"
)

// MITMHandler handles MITM proxy connections
type MITMHandler struct {
	proxy         *TransparentProxy
	mitmManager   *mitm.Manager
	logger        *slog.Logger
}

// NewMITMHandler creates a new MITM handler
func NewMITMHandler(proxy *TransparentProxy, mitmManager *mitm.Manager, logger *slog.Logger) *MITMHandler {
	return &MITMHandler{
		proxy:       proxy,
		mitmManager: mitmManager,
		logger:      logger,
	}
}

// HandleConnection handles a MITM connection for HTTPS traffic
func (h *MITMHandler) HandleConnection(clientConn net.Conn, originalDst OriginalDst) error {
	if !h.mitmManager.IsEnabled() {
		return fmt.Errorf("MITM is not enabled")
	}

	handler := h.mitmManager.ConnectionHandler(h.proxy.upstream)
	return handler.HandleConnection(clientConn, originalDst.IP, originalDst.Port)
}

// IsEnabled returns whether MITM handling is enabled
func (h *MITMHandler) IsEnabled() bool {
	return h.mitmManager != nil && h.mitmManager.IsEnabled()
}
