DO $$ BEGIN
  RAISE EXCEPTION 'irreversible migration: pgRouting removal requires OSM reimport';
END $$;
