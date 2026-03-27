SELECT remove_compression_policy('health_monitoring_samples', if_exists => true);
DROP TABLE IF EXISTS health_monitoring_samples CASCADE;
