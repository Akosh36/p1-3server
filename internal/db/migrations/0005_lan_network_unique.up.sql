-- Phase 9: internal/discovery auto-creates one lan_networks row per
-- observed switch (type='wired') and per observed SSID (type='wireless_local'),
-- upserted the same idempotent way upsertSwitchPorts already does for
-- switch_ports — via a real unique constraint + ON CONFLICT, not a racy
-- select-then-insert.
ALTER TABLE lan_networks ADD CONSTRAINT lan_networks_type_name_key UNIQUE (type, name);
