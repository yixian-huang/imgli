package mail

import (
	"errors"
	"strings"
	"testing"
)

func TestExplainSendError(t *testing.T) {
	cases := []struct {
		in   error
		want string
	}{
		{ErrNotConfigured, "请填写 SMTP 服务器"},
		{errors.New("530 5.7.1 Authentication required"), "服务器要求登录"},
		{errors.New(`535 5.7.8 Error: authentication failed`), "用户名或密码被拒绝"},
		{errors.New("535 5.7.8 Username and Password not accepted"), "用户名或密码被拒绝"},
		{errors.New("530 5.7.0 Must issue a STARTTLS command first"), "加密方式和端口不匹配"},
		{errors.New("tls: first record does not look like a TLS handshake"), "加密方式和端口不匹配"},
		{errors.New("x509: certificate is valid for smtp.example, not smtp.lark"), "加密方式和端口不匹配"},
		{errors.New("dial tcp: i/o timeout"), "连不上 SMTP 服务器"},
		{errors.New("dial tcp 1.2.3.4:465: connect: connection refused"), "连不上 SMTP 服务器"},
		{errors.New("dial tcp: lookup smtp.larksuite.com: no such host"), "SMTP 服务器地址无法解析"},
		{errors.New("550 5.7.1 Sender address rejected"), "发件人地址不被接受"},
		{errors.New("unencrypted connection"), "填了用户名就不能用「无加密」"},
		{errors.New("weird smtp boom"), "发送失败:weird smtp boom"},
	}
	for _, tc := range cases {
		got := ExplainSendError(tc.in)
		if !strings.Contains(got, tc.want) {
			t.Errorf("ExplainSendError(%q) = %q, 应含 %q", tc.in, got, tc.want)
		}
	}
	if ExplainSendError(nil) != "" {
		t.Error("nil 应空串")
	}
}
