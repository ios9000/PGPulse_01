ALTER TABLE alert_rules
    ADD COLUMN IF NOT EXISTS consecutive_points_required INT NOT NULL DEFAULT 0;

COMMENT ON COLUMN alert_rules.consecutive_points_required IS
    'Minimum consecutive forecast points that must cross threshold before alert fires. 0 = use global default.';
