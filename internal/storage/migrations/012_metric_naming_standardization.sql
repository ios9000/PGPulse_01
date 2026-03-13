-- MN_01: Metric naming standardization
-- Renames pgpulse.* → pg.*, fixes OS metric prefix, renames os.diskstat.* → os.disk.*
-- Covers: metrics, alert_rules, ml_baseline_snapshots

BEGIN;

-- 1. Bulk rename pgpulse.* → pg.* in metrics table (except OS metrics)
UPDATE metrics
SET metric = 'pg.' || substring(metric FROM 9)
WHERE metric LIKE 'pgpulse.%'
  AND metric NOT LIKE 'pgpulse.os.%';

-- 2. Fix OS metrics from SQL path: pgpulse.os.* → os.*
UPDATE metrics
SET metric = substring(metric FROM 9)
WHERE metric LIKE 'pgpulse.os.%';

-- 3. Rename os.diskstat.* → os.disk.*
UPDATE metrics
SET metric = replace(metric, 'os.diskstat.', 'os.disk.')
WHERE metric LIKE 'os.diskstat.%';

-- 4. Specific diskstat unit renames (read_kb → read_bytes_per_sec, write_kb → write_bytes_per_sec)
UPDATE metrics SET metric = 'os.disk.read_bytes_per_sec'
WHERE metric IN ('os.disk.read_kb', 'os.diskstat.read_kb');

UPDATE metrics SET metric = 'os.disk.write_bytes_per_sec'
WHERE metric IN ('os.disk.write_kb', 'os.diskstat.write_kb');

-- 5. Alert rules — same renames
UPDATE alert_rules
SET metric = 'pg.' || substring(metric FROM 9)
WHERE metric LIKE 'pgpulse.%'
  AND metric NOT LIKE 'pgpulse.os.%';

UPDATE alert_rules
SET metric = substring(metric FROM 9)
WHERE metric LIKE 'pgpulse.os.%';

UPDATE alert_rules
SET metric = replace(metric, 'os.diskstat.', 'os.disk.')
WHERE metric LIKE 'os.diskstat.%';

-- 6. ML baseline snapshots — same renames
UPDATE ml_baseline_snapshots
SET metric_key = 'pg.' || substring(metric_key FROM 9)
WHERE metric_key LIKE 'pgpulse.%'
  AND metric_key NOT LIKE 'pgpulse.os.%';

UPDATE ml_baseline_snapshots
SET metric_key = substring(metric_key FROM 9)
WHERE metric_key LIKE 'pgpulse.os.%';

UPDATE ml_baseline_snapshots
SET metric_key = replace(metric_key, 'os.diskstat.', 'os.disk.')
WHERE metric_key LIKE 'os.diskstat.%';

COMMIT;
