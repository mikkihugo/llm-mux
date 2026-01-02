package management

import "github.com/gin-gonic/gin"

func (h *Handler) GetSwitchProject(c *gin.Context) {
	respondOK(c, gin.H{"switch-project": h.cfg.QuotaExceeded.SwitchProject})
}
func (h *Handler) PutSwitchProject(c *gin.Context) {
	value, ok := h.bindBoolValue(c)
	if !ok {
		return
	}
	h.cfg.QuotaExceeded.SwitchProject = value
	if !h.persistSilent() {
		respondInternalError(c, "failed to save config")
		return
	}
	respondOK(c, gin.H{"switch-project": h.cfg.QuotaExceeded.SwitchProject})
}

func (h *Handler) GetSwitchPreviewModel(c *gin.Context) {
	respondOK(c, gin.H{"switch-preview-model": h.cfg.QuotaExceeded.SwitchPreviewModel})
}
func (h *Handler) PutSwitchPreviewModel(c *gin.Context) {
	value, ok := h.bindBoolValue(c)
	if !ok {
		return
	}
	h.cfg.QuotaExceeded.SwitchPreviewModel = value
	if !h.persistSilent() {
		respondInternalError(c, "failed to save config")
		return
	}
	respondOK(c, gin.H{"switch-preview-model": h.cfg.QuotaExceeded.SwitchPreviewModel})
}
