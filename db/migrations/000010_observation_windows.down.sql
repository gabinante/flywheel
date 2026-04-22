-- Reverse migration: Remove observation tables.
DROP TABLE IF EXISTS signal_rules;
DROP TABLE IF EXISTS attributions;
DROP TABLE IF EXISTS signals;
DROP TABLE IF EXISTS observation_windows;
