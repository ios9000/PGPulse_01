-- Expand role system from 2 roles (admin/viewer) to 4 permission-based roles.
-- Migrate existing users: admin → super_admin, viewer → dba.

-- Drop the existing CHECK constraint on role column.
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_role_check;

-- Migrate existing roles to new role names.
UPDATE users SET role = 'super_admin' WHERE role = 'admin';
UPDATE users SET role = 'dba' WHERE role = 'viewer';

-- Add new CHECK constraint with expanded roles.
ALTER TABLE users ADD CONSTRAINT users_role_check
    CHECK (role IN ('super_admin', 'roles_admin', 'dba', 'app_admin'));

-- Add active flag and last login tracking.
ALTER TABLE users ADD COLUMN IF NOT EXISTS active BOOLEAN NOT NULL DEFAULT true;
ALTER TABLE users ADD COLUMN IF NOT EXISTS last_login TIMESTAMPTZ;
