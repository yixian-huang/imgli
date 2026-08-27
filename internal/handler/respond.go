// Package handler 传输层公共件：lsky 兼容响应信封、错误码、基础中间件。
package handler

import (
	"encoding/json"
	"io"
	"net/http"
)

// 机器可读错误码——前端上传卡片按码渲染失败态（见 spec §5）。
const (
	CodeFileTooLarge       = "file_too_large"
	CodeQuotaExceeded      = "quota_exceeded"
	CodeExtNotAllowed      = "ext_not_allowed"
	CodeRateLimited        = "rate_limited"
	CodeModerationRejected = "moderation_rejected"
	CodeUnauthorized       = "unauthorized"
	CodeNotFound           = "not_found"
	CodeInvalidRequest     = "invalid_request"
	CodeInternal           = "internal_error"
	CodeForbidden          = "forbidden"
	CodeGone               = "resource_gone"
	CodeBandwidthExceeded  = "bandwidth_exceeded"
	// 用户组上传选项限制（上传 / 改图）。
	CodeExpiresOverGroup  = "expires_over_group"
	CodeMaxViewsOverGroup = "max_views_over_group"
	// 魔数已认出 HEIF，但本构建没有可用解码器（纯 Go / 无 libheif）。
	CodeHeicUnsupported = "heic_unsupported"
)

// MaxExpiresInSec 有效期上限(1 年):既作业务上限(过期用于临时分享无需多年),又防
// time.Duration(sec)*time.Second 整数溢出(sec>~9.2e9 时越界→负值→过去时间→立即清理,codex 评审)。
const MaxExpiresInSec = 366 * 24 * 60 * 60

type envelope struct {
	Status  bool   `json:"status"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

func writeJSON(w http.ResponseWriter, httpStatus int, e envelope) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(httpStatus)
	json.NewEncoder(w).Encode(e)
}

func OK(w http.ResponseWriter, data any) {
	writeJSON(w, http.StatusOK, envelope{Status: true, Message: "ok", Data: data})
}

func Fail(w http.ResponseWriter, httpStatus int, code, msg string) {
	writeJSON(w, httpStatus, envelope{Status: false, Message: msg, Data: map[string]string{"code": code}})
}

// DecodeJSON 解析请求体 JSON（上限 1MB）。
func DecodeJSON(r *http.Request, dst any) error {
	return json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(dst)
}
