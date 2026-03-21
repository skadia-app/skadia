-- 000202_device_data_repair.down.sql
-- Datenreparatur ist nicht sinnvoll rückgängig machbar — Down-Migration ist ein No-Op.
-- Die alten Werte sind nicht rekonstruierbar, da sie aus SDK-Bugs stammten.
SELECT 1;
