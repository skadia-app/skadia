CREATE TABLE developer_field_definitions (
    id                      SERIAL PRIMARY KEY,
    application_id          BYTEA,
    developer_data_index    SMALLINT NOT NULL,
    field_definition_number SMALLINT NOT NULL,
    field_name              TEXT NOT NULL,
    units                   TEXT,
    native_mesg_num         SMALLINT,
    native_field_num        SMALLINT,
    fit_base_type           SMALLINT,
    scale                   SMALLINT,
    "offset"                SMALLINT,
    created_at              TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (application_id, developer_data_index, field_definition_number)
);
