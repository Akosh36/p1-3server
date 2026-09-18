ALTER TABLE backend_metrics DROP COLUMN IF EXISTS gpu_mem_percent;
ALTER TABLE backend_metrics DROP COLUMN IF EXISTS gpu_percent;

ALTER TABLE system_metrics DROP COLUMN IF EXISTS gpu_mem_percent;
ALTER TABLE system_metrics DROP COLUMN IF EXISTS gpu_percent;
