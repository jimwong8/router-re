package auth

import "testing"

func TestHashAndVerify(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if hash == "correct horse battery staple" {
		t.Fatal("password not hashed")
	}
	if !VerifyPassword(hash, "correct horse battery staple") {
		t.Fatal("verify failed for correct password")
	}
	if VerifyPassword(hash, "wrong") {
		t.Fatal("verify succeeded for wrong password")
	}
}

func TestHashUnique(t *testing.T) {
	a, _ := HashPassword("same")
	b, _ := HashPassword("same")
	if a == b {
		t.Fatal("two hashes of same password should differ (random salt)")
	}
}

func TestVerifyBadHash(t *testing.T) {
	if VerifyPassword("not-a-hash", "pw") {
		t.Fatal("verify succeeded on malformed hash")
	}
	if VerifyPassword("$argon2id$v=19$m=1,t=1,p=1$$$$", "pw") {
		t.Fatal("verify succeeded on truncated hash")
	}
}
