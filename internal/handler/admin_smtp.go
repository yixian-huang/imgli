package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/yixian-huang/imgli/internal/apperr"
	"github.com/yixian-huang/imgli/internal/mail"
)

// TestSMTP POST /api/v1/admin/settings/smtp/test {to, smtp?}
// smtp 缺省时用已保存配置；带 smtp 则用表单覆盖（掩码密码仍按 host+username 解析）。
func (h *AdminHandlers) TestSMTP(w http.ResponseWriter, r *http.Request) {
	var req struct {
		To   string       `json:"to"`
		SMTP *mail.Config `json:"smtp"`
	}
	if err := DecodeJSON(r, &req); err != nil || !strings.Contains(req.To, "@") {
		Fail(w, http.StatusBadRequest, CodeInvalidRequest, "收件人邮箱无效")
		return
	}
	var err error
	if req.SMTP != nil {
		cfg, perr := h.D.Adm.PrepareSMTP(*req.SMTP)
		if perr != nil {
			if apperr.IsClient(perr) {
				Fail(w, http.StatusBadRequest, CodeInvalidRequest, perr.Error())
				return
			}
			Fail(w, http.StatusInternalServerError, CodeInternal, "服务器内部错误")
			return
		}
		err = h.D.Mail.SendWithConfig(cfg, req.To, "SMTP 测试邮件", "<p>看到这封邮件说明 SMTP 配置可用。</p>")
	} else {
		err = h.D.Mail.Send(req.To, "SMTP 测试邮件", "<p>看到这封邮件说明 SMTP 配置可用。</p>")
	}
	switch {
	case err == nil:
		actor := PrincipalFrom(r).User
		h.D.Adm.Audit(&actor.ID, "admin", "smtp_test", map[string]any{"to": req.To}, ClientIP(r))
		OK(w, map[string]any{})
	case errors.Is(err, mail.ErrNotConfigured):
		msg := "SMTP 未配置,请先保存邮件设置"
		if req.SMTP != nil {
			msg = "请填写 SMTP 服务器"
		}
		Fail(w, http.StatusBadRequest, CodeInvalidRequest, msg)
	default:
		Fail(w, http.StatusBadRequest, CodeInvalidRequest, mail.ExplainSendError(err))
	}
}
