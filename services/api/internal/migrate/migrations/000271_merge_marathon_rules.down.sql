-- Revert: delete merged rule (individual rules cannot be recreated without tag_ids)
DELETE FROM automation_rules WHERE name = 'Laufdistanz' AND is_system = false;
