-- Phase 6: an admin can end a capture session outright (no further
-- auto-continuation), distinct from an auto-rotation ('rotated') or a
-- download-triggered rotation ('downloaded') — both of those always start
-- a fresh recording segment right after, this one doesn't.
ALTER TYPE capture_status ADD VALUE 'stopped';
