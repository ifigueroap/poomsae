-- Rename the remaining kata identifiers. Embedding artifacts all predate the
-- poomsae digest and recipe names, so they are cleared for regeneration.
ALTER TABLE external_field_mappings RENAME COLUMN kata_field TO poomsae_field;
ALTER TABLE external_field_mappings
  RENAME CONSTRAINT external_field_mappings_kata_field_check TO external_field_mappings_poomsae_field_check;
ALTER TABLE external_field_states RENAME COLUMN conflict_kata TO conflict_poomsae;
ALTER TABLE external_field_states
  RENAME CONSTRAINT external_field_states_conflict_kata_check TO external_field_states_conflict_poomsae_check;

ALTER TEXT SEARCH CONFIGURATION kata_simple_unaccent RENAME TO poomsae_simple_unaccent;

CREATE OR REPLACE FUNCTION rebuild_issue_search(p_issue_id BIGINT) RETURNS void LANGUAGE plpgsql AS $$
BEGIN
  INSERT INTO issues_search (issue_id, tsv)
    SELECT id, to_tsvector('poomsae_simple_unaccent', title || ' ' || coalesce(body, ''))
    FROM issues WHERE id = p_issue_id
    ON CONFLICT (issue_id) DO UPDATE SET tsv = EXCLUDED.tsv;
END $$;

DELETE FROM federation_embedding_artifacts;
