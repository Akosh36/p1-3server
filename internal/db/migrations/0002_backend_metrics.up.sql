-- Phase 5: real host metrics (CPU/RAM/disk/network) for backend servers,
-- pushed by a small agent binary (cmd/backendagentd) running on each
-- backend, not polled by the control-plane. agent_token authenticates that
-- push (see internal/httpapi's handleAgentMetrics) — nullable because
-- backends created before this migration don't have one yet until an admin
-- regenerates it from the Servers page.
ALTER TABLE backend_servers ADD COLUMN agent_token TEXT UNIQUE;

CREATE TABLE backend_metrics (
    time                TIMESTAMPTZ NOT NULL DEFAULT now(),
    backend_server_id  BIGINT NOT NULL REFERENCES backend_servers(id) ON DELETE CASCADE,
    cpu_percent         NUMERIC,
    mem_percent         NUMERIC,
    disk_percent        NUMERIC,
    disk_read_bps       BIGINT,
    disk_write_bps      BIGINT,
    net_in_bps          BIGINT,
    net_out_bps         BIGINT
);
CREATE INDEX idx_backend_metrics_time ON backend_metrics(time DESC);
CREATE INDEX idx_backend_metrics_backend ON backend_metrics(backend_server_id, time DESC);
