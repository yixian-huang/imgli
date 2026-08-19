package mail

import (
	"errors"
	"fmt"
	"net/smtp"
	"strings"
)

// pickSMTPAuth 按 EHLO AUTH 能力选机制。飞书/Lark/163 等常只宣 LOGIN；
// 两边都宣时走 PLAIN（RFC 标准）。未宣 AUTH 时仍试 PLAIN。
func pickSMTPAuth(advertised bool, param string) string {
	if !advertised {
		return "PLAIN"
	}
	up := " " + strings.ToUpper(param) + " "
	hasPlain := strings.Contains(up, " PLAIN ")
	hasLogin := strings.Contains(up, " LOGIN ")
	if hasLogin && !hasPlain {
		return "LOGIN"
	}
	if hasPlain {
		return "PLAIN"
	}
	if hasLogin {
		return "LOGIN"
	}
	return "PLAIN"
}

func smtpAuthenticate(c *smtp.Client, cfg Config) error {
	if cfg.Username == "" {
		return nil
	}
	ok, param := c.Extension("AUTH")
	if pickSMTPAuth(ok, param) == "LOGIN" {
		return c.Auth(smtpLoginAuth(cfg.Username, cfg.Password))
	}
	return c.Auth(smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host))
}

type loginAuth struct {
	username, password string
	step               int
}

func smtpLoginAuth(username, password string) smtp.Auth {
	return &loginAuth{username: username, password: password}
}

func isLocalhost(name string) bool {
	return name == "localhost" || name == "127.0.0.1" || name == "::1"
}

func (a *loginAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	// 与 smtp.PlainAuth 同纪律：非 TLS 且非本机则拒发凭据。
	if !server.TLS && !isLocalhost(server.Name) {
		return "", nil, errors.New("unencrypted connection")
	}
	return "LOGIN", nil, nil
}

func (a *loginAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if !more {
		return nil, nil
	}
	a.step++
	switch a.step {
	case 1:
		return []byte(a.username), nil
	case 2:
		return []byte(a.password), nil
	default:
		return nil, fmt.Errorf("unexpected SMTP LOGIN step %d: %q", a.step, fromServer)
	}
}
