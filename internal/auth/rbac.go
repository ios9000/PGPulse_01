package auth

// Permission represents an action a user can perform.
type Permission string

const (
	PermUserManagement     Permission = "user_management"
	PermInstanceManagement Permission = "instance_management"
	PermAlertManagement    Permission = "alert_management"
	PermViewAll            Permission = "view_all"
	PermSelfManagement     Permission = "self_management"
)

// Role represents a user role in the system.
type Role string

const (
	RoleSuperAdmin Role = "super_admin"
	RoleRolesAdmin Role = "roles_admin"
	RoleDBA        Role = "dba"
	RoleAppAdmin   Role = "app_admin"
)

// ValidRoles lists all valid role values.
var ValidRoles = []Role{RoleSuperAdmin, RoleRolesAdmin, RoleDBA, RoleAppAdmin}

// RolePermissions maps each role to its granted permissions.
var RolePermissions = map[Role][]Permission{
	RoleSuperAdmin: {PermUserManagement, PermInstanceManagement, PermAlertManagement, PermViewAll, PermSelfManagement},
	RoleRolesAdmin: {PermUserManagement, PermViewAll, PermSelfManagement},
	RoleDBA:        {PermInstanceManagement, PermAlertManagement, PermViewAll, PermSelfManagement},
	RoleAppAdmin:   {PermAlertManagement, PermViewAll, PermSelfManagement},
}

// HasPermission reports whether the given role has the specified permission.
func HasPermission(role Role, perm Permission) bool {
	perms, ok := RolePermissions[role]
	if !ok {
		return false
	}
	for _, p := range perms {
		if p == perm {
			return true
		}
	}
	return false
}

// PermissionsForRole returns the permission strings for a role.
func PermissionsForRole(role Role) []string {
	perms, ok := RolePermissions[role]
	if !ok {
		return nil
	}
	result := make([]string, len(perms))
	for i, p := range perms {
		result[i] = string(p)
	}
	return result
}

// ValidRole reports whether role is a known role string.
func ValidRole(role string) bool {
	for _, r := range ValidRoles {
		if string(r) == role {
			return true
		}
	}
	return false
}
