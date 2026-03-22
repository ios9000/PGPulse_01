-- 017_recommendation_rca_bridge.sql — Bridge RCA incidents to remediation recommendations

-- Add new columns to remediation_recommendations.
ALTER TABLE remediation_recommendations
    ADD COLUMN IF NOT EXISTS source TEXT NOT NULL DEFAULT 'alert',
    ADD COLUMN IF NOT EXISTS urgency_score FLOAT8 NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS incident_ids BIGINT[] NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS last_incident_at TIMESTAMPTZ;

-- Partial unique index for RCA-sourced recommendations (one active per rule+instance+source).
-- The existing idx_remediation_active_unique covers (rule_id, instance_id) WHERE status='active',
-- which already prevents duplicates. No additional unique index needed for source.

-- GIN index for incident_ids array lookups.
CREATE INDEX IF NOT EXISTS idx_remediation_incident_ids
    ON remediation_recommendations USING GIN (incident_ids)
    WHERE array_length(incident_ids, 1) > 0;

-- Index for source filtering.
CREATE INDEX IF NOT EXISTS idx_remediation_source
    ON remediation_recommendations (source, created_at DESC);

-- Backfill existing rows: set urgency_score from priority.
UPDATE remediation_recommendations
SET urgency_score = CASE priority
    WHEN 'action_required' THEN 5.0
    WHEN 'suggestion' THEN 3.0
    WHEN 'info' THEN 1.0
    ELSE 2.0
END
WHERE urgency_score = 0;
