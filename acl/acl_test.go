package acl

import (
	"testing"
)

func TestACLDefaultUser(t *testing.T) {
	m := NewManager()

	// Default user is enabled and nopass
	if !m.Authenticate("default", "") {
		t.Fatalf("expected default user to authenticate without password")
	}

	ok, _ := m.CheckPermission("default", "GET", "mykey")
	if !ok {
		t.Fatalf("expected default user to be allowed GET")
	}

	ok, _ = m.CheckPermission("default", "FLUSHDB", "")
	if !ok {
		t.Fatalf("expected default user to be allowed FLUSHDB")
	}
}

func TestACLSetUserAndPassword(t *testing.T) {
	m := NewManager()

	// Create user 'alice' with password 'secretPass123'
	err := m.SetUser("alice", []string{"on", ">secretPass123", "+@read", "~cached:*"})
	if err != nil {
		t.Fatalf("unexpected error setting user: %v", err)
	}

	// Bad password
	if m.Authenticate("alice", "wrongpassword") {
		t.Fatalf("expected auth to fail with wrong password")
	}

	// Correct password
	if !m.Authenticate("alice", "secretPass123") {
		t.Fatalf("expected auth to succeed with correct password")
	}

	// Allowed command GET on matching key
	ok, _ := m.CheckPermission("alice", "GET", "cached:user:1")
	if !ok {
		t.Fatalf("expected GET on cached:user:1 to be allowed")
	}

	// Allowed command GET on non-matching key
	ok, reason := m.CheckPermission("alice", "GET", "other:user:1")
	if ok {
		t.Fatalf("expected GET on other:user:1 to be forbidden, got ok (reason: %s)", reason)
	}

	// Forbidden command SET (write category not granted)
	ok, _ = m.CheckPermission("alice", "SET", "cached:user:1")
	if ok {
		t.Fatalf("expected SET to be forbidden")
	}
}

func TestACLDelUser(t *testing.T) {
	m := NewManager()
	_ = m.SetUser("bob", []string{"on", "nopass"})

	users := m.ListUsers()
	if len(users) != 2 {
		t.Fatalf("expected 2 users, got %v", users)
	}

	deleted := m.DelUser("bob")
	if deleted != 1 {
		t.Fatalf("expected 1 user deleted, got %d", deleted)
	}

	// Cannot delete default
	deletedDefault := m.DelUser("default")
	if deletedDefault != 0 {
		t.Fatalf("expected default user not to be deleted")
	}
}

func TestACLDescribeUsers(t *testing.T) {
	m := NewManager()
	desc := m.DescribeUsers()
	if len(desc) != 1 || desc[0] != "user default on nopass ~* +@all" {
		t.Fatalf("unexpected describe: %v", desc)
	}
}
