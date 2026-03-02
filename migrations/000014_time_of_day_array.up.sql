-- time_of_day: TEXT → TEXT[] (multiple periods for activities spanning dawn/dusk)
ALTER TABLE activities
  ALTER COLUMN time_of_day DROP DEFAULT,
  ALTER COLUMN time_of_day TYPE TEXT[] USING CASE
    WHEN time_of_day IS NOT NULL THEN ARRAY[time_of_day]
    ELSE NULL
  END;
