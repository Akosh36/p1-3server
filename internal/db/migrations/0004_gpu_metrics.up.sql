-- GPU metrics (Phase 8): nullable everywhere, because most hosts running
-- this platform have no GPU at all. internal/hostmetrics only ever writes a
-- real nvidia-smi reading here or leaves the column NULL — never a fake 0 —
-- so "no GPU" and "GPU present but currently idle" stay distinguishable.
ALTER TABLE system_metrics ADD COLUMN gpu_percent NUMERIC;
ALTER TABLE system_metrics ADD COLUMN gpu_mem_percent NUMERIC;

ALTER TABLE backend_metrics ADD COLUMN gpu_percent NUMERIC;
ALTER TABLE backend_metrics ADD COLUMN gpu_mem_percent NUMERIC;
