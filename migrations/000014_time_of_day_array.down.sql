-- Rollback: TEXT[] → TEXT (take first element)
ALTER TABLE activities
  ALTER COLUMN time_of_day TYPE TEXT USING time_of_day[1];
