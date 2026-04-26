-- not reversible: alignment of two settings is one-way; the original
-- divergent state cannot be reconstructed once the two columns have been
-- forced to match. Treat as a no-op for migrate-down.
SELECT 1;
