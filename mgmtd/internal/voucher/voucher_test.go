package voucher

import (
	"strings"
	"testing"
)

func validVoucher() Voucher {
	return Voucher{
		Code:        "ABC123",
		Password:    "secret12",
		Minutes:     60,
		TrafficMB:   100,
		DeviceLimit: 1,
		CreatedAt:   1000,
	}
}

func TestValidateNormalizesLowercase(t *testing.T) {
	v := validVoucher()
	v.Code = "abc123"
	if err := Validate(v); err != nil {
		t.Fatalf("lowercase code should be accepted: %v", err)
	}
	// pointer mutation check
	vp := validVoucher()
	vp.Code = "abc123"
	if err := Validate(&vp); err != nil {
		t.Fatalf("lowercase code should be accepted: %v", err)
	}
	if vp.Code != "ABC123" {
		t.Fatalf("Code should be normalized to ABC123, got %q", vp.Code)
	}
}

func TestValidateRejectsBadChars(t *testing.T) {
	v := validVoucher()
	v.Code = "ABC-123"
	if err := Validate(v); err == nil {
		t.Fatal("code with '-' must fail")
	}
}

func TestValidateRejectsShortCode(t *testing.T) {
	v := validVoucher()
	v.Code = "AB"
	if err := Validate(v); err == nil {
		t.Fatal("short code must fail")
	}
}

func TestGenerateCodeNoConfusable(t *testing.T) {
	for i := 0; i < 50; i++ {
		code, err := GenerateCode(16)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range code {
			if strings.ContainsRune("0O1Il", c) {
				t.Fatalf("generated code contains confusable char %c: %s", c, code)
			}
		}
	}
}

func TestGenerateBatchUnique(t *testing.T) {
	codes, err := GenerateBatch(1000, 12)
	if err != nil {
		t.Fatal(err)
	}
	if len(codes) != 1000 {
		t.Fatalf("expected 1000 codes, got %d", len(codes))
	}
	seen := map[string]bool{}
	for _, c := range codes {
		if seen[c] {
			t.Fatalf("duplicate code %s", c)
		}
		seen[c] = true
	}
}

func TestRedeemSameErrorForMissingAndBadPassword(t *testing.T) {
	// missing card: nil voucher
	_, _, err1 := Redeem(nil, "pw", 1000)
	// bad password
	v := validVoucher()
	_, _, err2 := Redeem(&v, "wrongpw", 1000)
	if err1 == nil || err2 == nil {
		t.Fatal("both must fail")
	}
	if err1.Error() != err2.Error() {
		t.Fatalf("error messages must be identical to prevent enumeration: %q vs %q", err1.Error(), err2.Error())
	}
}

func TestRedeemExpired(t *testing.T) {
	v := validVoucher()
	v.ExpiresAt = 100
	_, _, err := Redeem(&v, v.Password, 200)
	if err == nil {
		t.Fatal("expired card must fail")
	}
}

func TestRedeemSetsUsedAndPreservesUsedAt(t *testing.T) {
	v := validVoucher()
	mins, traffic, err := Redeem(&v, v.Password, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if mins != v.Minutes || traffic != v.TrafficMB {
		t.Fatalf("redeem returned %d/%d, want %d/%d", mins, traffic, v.Minutes, v.TrafficMB)
	}
	if !v.Used {
		t.Fatal("Used must be true after redeem")
	}
	firstUsedAt := v.UsedAt
	if firstUsedAt == 0 {
		t.Fatal("UsedAt must be set")
	}
	// repeat redeem: UsedAt must not be overwritten
	_, _, err = Redeem(&v, v.Password, 2000)
	if err != nil {
		// repeat might fail if used up; for unlimited-ish here minutes=60, remaining at 2000 = 60-16=44>0 so ok
		t.Fatalf("repeat redeem unexpected err: %v", err)
	}
	if v.UsedAt != firstUsedAt {
		t.Fatalf("UsedAt overwritten: %d vs %d", v.UsedAt, firstUsedAt)
	}
}

func TestRemainingMinutes(t *testing.T) {
	v := validVoucher()
	v.Used = true
	v.UsedAt = 1000
	v.Minutes = 60
	// 30 minutes later
	got := RemainingMinutes(v, 1000+30*60)
	if got != 30 {
		t.Fatalf("RemainingMinutes=%d, want 30", got)
	}
}

func TestMaskHidesPassword(t *testing.T) {
	v := validVoucher()
	v.Password = "secret12"
	m := Mask(v)
	if strings.Contains(m.Password, "secret12") {
		t.Fatal("mask must not leak full password")
	}
	if m.Password == v.Password {
		t.Fatal("mask must change password")
	}
}
