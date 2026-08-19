package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/yixian-huang/imgli/internal/apperr"
	"github.com/yixian-huang/imgli/internal/mail"
)

// PreviewMail POST /api/v1/admin/settings/mail/preview {kind, lang, templates?}
func (h *AdminHandlers) PreviewMail(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Kind      string          `json:"kind"`
		Lang      string          `json:"lang"`
		Templates *mail.Templates `json:"templates"`
	}
	if err := DecodeJSON(r, &req); err != nil {
		Fail(w, http.StatusBadRequest, CodeInvalidRequest, "请求体无效")
		return
	}
	kind, ok := mail.ParseKind(req.Kind)
	if !ok {
		Fail(w, http.StatusBadRequest, CodeInvalidRequest, mail.ErrCopyKindInvalid.Error())
		return
	}
	if req.Templates != nil {
		tpl := req.Templates.Normalize()
		if err := mail.ValidateTemplates(tpl); err != nil {
			Fail(w, http.StatusBadRequest, CodeInvalidRequest, err.Error())
			return
		}
		req.Templates = &tpl
	}
	sub, html, err := h.D.Mail.Preview(kind, req.Lang, req.Templates)
	if err != nil {
		if apperr.IsClient(err) {
			Fail(w, http.StatusBadRequest, CodeInvalidRequest, err.Error())
			return
		}
		Fail(w, http.StatusInternalServerError, CodeInternal, "服务器内部错误")
		return
	}
	OK(w, map[string]any{"subject": sub, "html": html})
}

// TestMail POST /api/v1/admin/settings/mail/test {to, kind, lang, templates?, smtp?}
func (h *AdminHandlers) TestMail(w http.ResponseWriter, r *http.Request) {
	var req struct {
		To        string          `json:"to"`
		Kind      string          `json:"kind"`
		Lang      string          `json:"lang"`
		Templates *mail.Templates `json:"templates"`
		SMTP      *mail.Config    `json:"smtp"`
	}
	if err := DecodeJSON(r, &req); err != nil || !strings.Contains(req.To, "@") {
		Fail(w, http.StatusBadRequest, CodeInvalidRequest, "收件人邮箱无效")
		return
	}
	kind, ok := mail.ParseKind(req.Kind)
	if !ok {
		Fail(w, http.StatusBadRequest, CodeInvalidRequest, mail.ErrCopyKindInvalid.Error())
		return
	}
	if req.Templates != nil {
		tpl := req.Templates.Normalize()
		if err := mail.ValidateTemplates(tpl); err != nil {
			Fail(w, http.StatusBadRequest, CodeInvalidRequest, err.Error())
			return
		}
		req.Templates = &tpl
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
		err = h.D.Mail.SendKindWithConfig(cfg, req.To, kind, req.Lang, req.Templates)
	} else {
		err = h.D.Mail.SendKind(req.To, kind, req.Lang, req.Templates)
	}
	switch {
	case err == nil:
		actor := PrincipalFrom(r).User
		h.D.Adm.Audit(&actor.ID, "admin", "mail_template_test", map[string]any{"to": req.To, "kind": req.Kind}, ClientIP(r))
		OK(w, map[string]any{})
	case errors.Is(err, mail.ErrNotConfigured):
		Fail(w, http.StatusBadRequest, CodeInvalidRequest, "请填写 SMTP 服务器")
	case apperr.IsClient(err):
		Fail(w, http.StatusBadRequest, CodeInvalidRequest, err.Error())
	default:
		Fail(w, http.StatusBadRequest, CodeInvalidRequest, mail.ExplainSendError(err))
	}
}
