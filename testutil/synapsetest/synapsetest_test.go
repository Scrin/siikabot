//go:build e2e

package synapsetest

import (
	"context"
	"strings"
	"testing"
)

func TestStartAndRegister(t *testing.T) {
	s, err := Start()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Stop()

	account, err := s.Register(context.Background(), "alice", "password", false)
	if err != nil {
		t.Fatalf("Register() error = %v\n%s", err, s.Log())
	}
	if account.UserID != "@alice:localhost" || account.AccessToken == "" {
		t.Errorf("Register() = %+v", account)
	}
	if _, err := s.Register(context.Background(), "alice", "password", false); err == nil || !strings.Contains(err.Error(), "400") {
		t.Errorf("registering a taken name = %v, want it refused", err)
	}
}
