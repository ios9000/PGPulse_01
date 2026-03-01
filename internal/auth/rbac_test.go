package auth

import "testing"

func TestHasRole_AdminHasAdmin(t *testing.T) {
	if !HasRole(RoleAdmin, RoleAdmin) {
		t.Error("admin should satisfy admin requirement")
	}
}

func TestHasRole_AdminHasViewer(t *testing.T) {
	if !HasRole(RoleAdmin, RoleViewer) {
		t.Error("admin should satisfy viewer requirement")
	}
}

func TestHasRole_ViewerHasViewer(t *testing.T) {
	if !HasRole(RoleViewer, RoleViewer) {
		t.Error("viewer should satisfy viewer requirement")
	}
}

func TestHasRole_ViewerNotAdmin(t *testing.T) {
	if HasRole(RoleViewer, RoleAdmin) {
		t.Error("viewer should NOT satisfy admin requirement")
	}
}

func TestHasRole_UnknownRole(t *testing.T) {
	if HasRole("superuser", RoleAdmin) {
		t.Error("unknown role should return false")
	}
}

func TestValidRole(t *testing.T) {
	if !ValidRole(RoleAdmin) {
		t.Errorf("ValidRole(%q) = false, want true", RoleAdmin)
	}
	if !ValidRole(RoleViewer) {
		t.Errorf("ValidRole(%q) = false, want true", RoleViewer)
	}
	if ValidRole("unknown") {
		t.Error("ValidRole(unknown) = true, want false")
	}
}
