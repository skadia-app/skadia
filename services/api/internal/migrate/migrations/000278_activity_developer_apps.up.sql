CREATE TABLE activity_developer_apps (
    id             UUID  NOT NULL DEFAULT gen_random_uuid(),
    activity_id    UUID  NOT NULL REFERENCES activities(id) ON DELETE CASCADE,
    application_id BYTEA NOT NULL,
    app_name       TEXT,
    app_category   TEXT,
    serial_number  TEXT,
    device_id      UUID  REFERENCES devices(id) ON DELETE SET NULL,
    PRIMARY KEY (activity_id, application_id)
);

CREATE INDEX idx_activity_developer_apps_device
    ON activity_developer_apps(device_id) WHERE device_id IS NOT NULL;

CREATE INDEX idx_activity_developer_apps_activity
    ON activity_developer_apps(activity_id);
