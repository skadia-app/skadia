CREATE TABLE user_daily_features (
    user_id     UUID NOT NULL,
    date        DATE NOT NULL,
    activity_id UUID DEFAULT '00000000-0000-0000-0000-000000000000',
    feature_key TEXT NOT NULL,
    value_num   DOUBLE PRECISION,
    value_json  JSONB,
    computed_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (user_id, date, feature_key, activity_id)
);

CREATE INDEX idx_udf_daily ON user_daily_features (user_id, date)
    WHERE activity_id = '00000000-0000-0000-0000-000000000000';

CREATE INDEX idx_udf_activity ON user_daily_features (activity_id)
    WHERE activity_id != '00000000-0000-0000-0000-000000000000';

CREATE INDEX idx_udf_feature_key ON user_daily_features (user_id, feature_key, date);
