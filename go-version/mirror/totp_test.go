package mirror

import (
	"testing"
	"time"
)

func TestCurrentMFACodeAndSecretParsing(t *testing.T) {
	now := func() time.Time { return time.Unix(59, 0) }
	code, err := CurrentMFACode("", now)
	if err != nil || code != "" {
		t.Fatalf("unexpected empty code result: %q %v", code, err)
	}
	code, err = CurrentMFACode("654321", now)
	if err != nil || code != "654321" {
		t.Fatalf("unexpected static code: %q %v", code, err)
	}
	code, err = CurrentMFACode("JBSWY3DPEHPK3PXP", now)
	if err != nil || len(code) != 6 {
		t.Fatalf("unexpected totp code: %q %v", code, err)
	}
	uriCode, err := CurrentMFACode("otpauth://totp/Test?secret=JBSWY3DPEHPK3PXP", now)
	if err != nil || uriCode != code {
		t.Fatalf("unexpected uri code: %q %v", uriCode, err)
	}
	if _, err := CurrentMFACode("!!!!", now); err == nil {
		t.Fatal("expected invalid secret error")
	}
	secret, err := extractSecret("otpauth://totp/Test?secret=JBSW-Y3DP")
	if err != nil || secret != "JBSWY3DP" {
		t.Fatalf("unexpected secret parse: %q %v", secret, err)
	}
	if _, err := extractSecret("otpauth://totp/Test"); err == nil {
		t.Fatal("expected missing secret error")
	}
	if _, err := extractSecret("otpauth://%zz"); err == nil {
		t.Fatal("expected invalid uri error")
	}
	if _, err := generateTOTP("!!!!", now()); err == nil {
		t.Fatal("expected invalid base32 totp error")
	}
	if !isDigits("123456") || isDigits("12a456") {
		t.Fatal("unexpected digit test result")
	}
}
