package application_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/0furkancolak/hooklane/api/internal/application"
	"github.com/0furkancolak/hooklane/api/internal/testutil/memstore"
)

func TestPasswordHashing(t *testing.T) {
	hash, err := application.HashPassword("correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(hash, "$argon2id$v=19$") {
		t.Fatalf("hash format: %s", hash)
	}
	if !application.VerifyPassword("correct horse battery", hash) {
		t.Fatal("right password rejected")
	}
	if application.VerifyPassword("correct horse batter", hash) {
		t.Fatal("wrong password accepted")
	}
	again, _ := application.HashPassword("correct horse battery")
	if again == hash {
		t.Fatal("two hashes of one password are identical: salt is not random")
	}
	for _, bad := range []string{"", "plain", "$argon2id$v=19$m=999999999,t=1,p=1$AAAA$AAAA", "$argon2i$v=19$m=19456,t=2,p=1$AAAA$AAAA"} {
		if application.VerifyPassword("x", bad) {
			t.Fatalf("malformed hash %q verified", bad)
		}
	}
}

func TestSessionExpires(t *testing.T) {
	store := memstore.New()
	accounts := application.NewAccountService(store.Accounts(), store.Sessions(), store.Orgs(), store, true)
	auth := application.NewAuthenticator(store.Keys()).WithSessions(store.Sessions())
	now := time.Now()
	accounts.SetClock(func() time.Time { return now })
	auth.SetClock(func() time.Time { return now })

	signedIn, err := accounts.SignUp(context.Background(), "a@example.com", "correct horse battery", "")
	if err != nil {
		t.Fatal(err)
	}
	if signedIn.Account.Organization.Name != "a" {
		t.Fatalf("default organization name = %q", signedIn.Account.Organization.Name)
	}
	if _, err := auth.Authenticate(context.Background(), signedIn.Token); err != nil {
		t.Fatalf("fresh session: %v", err)
	}
	now = now.Add(application.SessionTTL + time.Second)
	if _, err := auth.Authenticate(context.Background(), signedIn.Token); !errors.Is(err, application.ErrUnauthorized) {
		t.Fatalf("expired session: %v", err)
	}
}
