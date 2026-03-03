export type Permission =
  | 'user_management'
  | 'instance_management'
  | 'alert_management'
  | 'view_all'
  | 'self_management'

export type Role = 'super_admin' | 'roles_admin' | 'dba' | 'app_admin'

const ROLE_PERMISSIONS: Record<Role, Permission[]> = {
  super_admin: ['user_management', 'instance_management', 'alert_management', 'view_all', 'self_management'],
  roles_admin: ['user_management', 'view_all', 'self_management'],
  dba: ['instance_management', 'alert_management', 'view_all', 'self_management'],
  app_admin: ['alert_management', 'view_all', 'self_management'],
}

export function hasPermission(role: Role, permission: Permission): boolean {
  const perms = ROLE_PERMISSIONS[role]
  return perms ? perms.includes(permission) : false
}

export function getPermissions(role: Role): Permission[] {
  return ROLE_PERMISSIONS[role] ?? []
}
