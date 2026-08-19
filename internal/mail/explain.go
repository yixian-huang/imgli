package mail

import (
	"errors"
	"strings"
)

// ExplainSendError 把 net/smtp 原文译成可操作中文。未知错误保留「发送失败:」前缀以便排查。
func ExplainSendError(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, ErrNotConfigured) {
		return "请填写 SMTP 服务器"
	}
	s := strings.ToLower(err.Error())
	switch {
	case strings.Contains(s, "starttls") && (strings.Contains(s, "530") || strings.Contains(s, "must issue")):
		return "加密方式和端口不匹配。465 请选 SSL，587 请选 STARTTLS"
	case strings.Contains(s, "tls:") || strings.Contains(s, "x509:") || strings.Contains(s, "handshake") ||
		strings.Contains(s, "first record does not look like"):
		return "加密方式和端口不匹配。465 请选 SSL，587 请选 STARTTLS"
	case strings.Contains(s, "535") || strings.Contains(s, "username and password not accepted") ||
		strings.Contains(s, "authentication failed") || strings.Contains(s, "invalid login"):
		return "用户名或密码被拒绝。飞书/Lark 请用完整邮箱 + 邮箱设置里的 IMAP/SMTP 密码，不是登录密码"
	case strings.Contains(s, "530") || strings.Contains(s, "authentication required") ||
		strings.Contains(s, "auth required"):
		return "服务器要求登录。用户名请填完整邮箱，密码用 IMAP/SMTP 专用密码（不是登录密码）"
	case strings.Contains(s, "no such host") || strings.Contains(s, "server misbehaving"):
		return "SMTP 服务器地址无法解析，请核对主机名"
	case strings.Contains(s, "i/o timeout") || strings.Contains(s, "timeout") ||
		strings.Contains(s, "connection refused") || strings.Contains(s, "connection reset") ||
		strings.Contains(s, "network is unreachable"):
		return "连不上 SMTP 服务器。请核对地址/端口，并确认本机出网未拦截 465/587"
	case strings.Contains(s, "550") || strings.Contains(s, "553") || strings.Contains(s, "sender") &&
		(strings.Contains(s, "reject") || strings.Contains(s, "not allowed") || strings.Contains(s, "denied")):
		return "发件人地址不被接受。公共邮箱的发件人须与登录用户名相同"
	case strings.Contains(s, "unencrypted connection"):
		return "填了用户名就不能用「无加密」。请改用 STARTTLS 或 SSL"
	default:
		return "发送失败:" + err.Error()
	}
}
