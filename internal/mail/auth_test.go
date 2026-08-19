package mail

import (
	"net/smtp"
	"strings"
	"testing"
)

func TestPickSMTPAuth(t *testing.T) {
	cases := []struct {
		advertised bool
		param      string
		want       string
	}{
		{false, "", "PLAIN"},
		{true, "PLAIN", "PLAIN"},
		{true, "LOGIN", "LOGIN"},
		{true, "PLAIN LOGIN", "PLAIN"},
		{true, "LOGIN PLAIN", "PLAIN"},
		{true, "login", "LOGIN"},
		{true, "CRAM-MD5", "PLAIN"},
	}
	for _, tc := range cases {
		if got := pickSMTPAuth(tc.advertised, tc.param); got != tc.want {
			t.Errorf("pickSMTPAuth(%v, %q) = %q, want %q", tc.advertised, tc.param, got, tc.want)
		}
	}
}

func TestLoginAuthChallenge(t *testing.T) {
	a := smtpLoginAuth("user@qqqu.de", "s3cret")
	mech, resp, err := a.Start(&smtp.ServerInfo{Name: "smtp.larksuite.com", TLS: true})
	if err != nil || mech != "LOGIN" || len(resp) != 0 {
		t.Fatalf("Start = %q %q %v", mech, resp, err)
	}
	u, err := a.Next([]byte("Username:"), true)
	if err != nil || string(u) != "user@qqqu.de" {
		t.Fatalf("username step = %q %v", u, err)
	}
	p, err := a.Next([]byte("Password:"), true)
	if err != nil || string(p) != "s3cret" {
		t.Fatalf("password step = %q %v", p, err)
	}
	if _, err := a.Next(nil, false); err != nil {
		t.Fatalf("done: %v", err)
	}
}

func TestLoginAuthRejectsPlaintext(t *testing.T) {
	a := smtpLoginAuth("u", "p")
	_, _, err := a.Start(&smtp.ServerInfo{Name: "smtp.larksuite.com", TLS: false})
	if err == nil || !strings.Contains(err.Error(), "unencrypted") {
		t.Fatalf("非 TLS 应拒绝, err = %v", err)
	}
}
