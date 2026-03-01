package auth

// Role constants.
const (
	RoleAdmin  = "admin"
	RoleViewer = "viewer"
)

// roleHierarchy maps each role to a numeric level.
// Higher number = more permissions.
var roleHierarchy = map[string]int{
	RoleViewer: 1,
	RoleAdmin:  2,
}

// HasRole reports whether userRole meets or exceeds the required role level.
func HasRole(userRole, requiredRole string) bool {
	userLevel, ok1 := roleHierarchy[userRole]
	requiredLevel, ok2 := roleHierarchy[requiredRole]
	if !ok1 || !ok2 {
		return false
	}
	return userLevel >= requiredLevel
}

// ValidRole reports whether role is a known role string.
func ValidRole(role string) bool {
	_, ok := roleHierarchy[role]
	return ok
}
