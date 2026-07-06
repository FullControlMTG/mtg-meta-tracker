package auth

import "testing"

func TestHashAndVerifyPassword(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	ok, err := VerifyPassword(hash, "correct horse battery staple")
	if err != nil || !ok {
		t.Fatalf("expected match, got ok=%v err=%v", ok, err)
	}
	bad, err := VerifyPassword(hash, "wrong password")
	if err != nil {
		t.Fatal(err)
	}
	if bad {
		t.Fatal("expected mismatch for wrong password")
	}
}

func TestTokenHashDeterministic(t *testing.T) {
	raw, hash, err := GenerateToken()
	if err != nil {
		t.Fatal(err)
	}
	if HashToken(raw) != hash {
		t.Fatal("HashToken should match GenerateToken's hash")
	}
	if raw == hash {
		t.Fatal("raw token must not equal its hash")
	}
}
