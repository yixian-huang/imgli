package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	appver "github.com/yixian-huang/imgli/internal/version"
)

type upgradeRequest struct {
	Confirm bool   `json:"confirm"`
	Tag     string `json:"tag"` // optional; empty = latest
}

// UpgradeSystem POST /api/v1/admin/system/upgrade
func (h *AdminHandlers) UpgradeSystem(w http.ResponseWriter, r *http.Request) {
	var req upgradeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		Fail(w, http.StatusBadRequest, CodeInvalidRequest, "请求体无效")
		return
	}
	res, err := appver.UpgradeBinary(r.Context(), appver.DefaultReleaseRepo, req.Tag, req.Confirm, nil)
	if err != nil {
		if errors.Is(err, appver.ErrUpgradeNoConfirm) {
			Fail(w, http.StatusBadRequest, CodeInvalidRequest, err.Error())
			return
		}
		if errors.Is(err, appver.ErrUpgradeDocker) {
			OK(w, map[string]any{
				"mode":    res.Mode,
				"message": res.Message,
				"from":    res.From,
				"error":   err.Error(),
			})
			return
		}
		Fail(w, http.StatusBadRequest, CodeInvalidRequest, err.Error())
		return
	}
	OK(w, res)
}
