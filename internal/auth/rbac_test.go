package auth

import "testing"

func TestHasPermission_SuperAdminHasAll(t *testing.T) {
	perms := []Permission{PermUserManagement, PermInstanceManagement, PermAlertManagement, PermViewAll, PermSelfManagement}
	for _, p := range perms {
		if !HasPermission(RoleSuperAdmin, p) {
			t.Errorf("super_admin should have permission %q", p)
		}
	}
}

func TestHasPermission_RolesAdminSubset(t *testing.T) {
	if !HasPermission(RoleRolesAdmin, PermUserManagement) {
		t.Error("roles_admin should have user_management")
	}
	if HasPermission(RoleRolesAdmin, PermInstanceManagement) {
		t.Error("roles_admin should NOT have instance_management")
	}
}

func TestHasPermission_DBASubset(t *testing.T) {
	if !HasPermission(RoleDBA, PermInstanceManagement) {
		t.Error("dba should have instance_management")
	}
	if !HasPermission(RoleDBA, PermAlertManagement) {
		t.Error("dba should have alert_management")
	}
	if HasPermission(RoleDBA, PermUserManagement) {
		t.Error("dba should NOT have user_management")
	}
}

func TestHasPermission_AppAdminSubset(t *testing.T) {
	if !HasPermission(RoleAppAdmin, PermAlertManagement) {
		t.Error("app_admin should have alert_management")
	}
	if HasPermission(RoleAppAdmin, PermInstanceManagement) {
		t.Error("app_admin should NOT have instance_management")
	}
	if HasPermission(RoleAppAdmin, PermUserManagement) {
		t.Error("app_admin should NOT have user_management")
	}
}

func TestHasPermission_UnknownRole(t *testing.T) {
	if HasPermission("superuser", PermViewAll) {
		t.Error("unknown role should return false")
	}
}

func TestPermissionsForRole(t *testing.T) {
	perms := PermissionsForRole(RoleSuperAdmin)
	if len(perms) != 5 {
		t.Errorf("PermissionsForRole(super_admin) = %d permissions, want 5", len(perms))
	}
	perms = PermissionsForRole("unknown")
	if perms != nil {
		t.Errorf("PermissionsForRole(unknown) = %v, want nil", perms)
	}
}

func TestValidRole(t *testing.T) {
	if !ValidRole(string(RoleSuperAdmin)) {
		t.Errorf("ValidRole(%q) = false, want true", RoleSuperAdmin)
	}
	if !ValidRole(string(RoleDBA)) {
		t.Errorf("ValidRole(%q) = false, want true", RoleDBA)
	}
	if ValidRole("unknown") {
		t.Error("ValidRole(unknown) = true, want false")
	}
	if ValidRole("admin") {
		t.Error("ValidRole(admin) = true, want false (legacy role)")
	}
}
