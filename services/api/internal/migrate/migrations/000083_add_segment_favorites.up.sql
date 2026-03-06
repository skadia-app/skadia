-- Create segment_favorites table for segment liking
CREATE TABLE segment_favorites (
    id BIGSERIAL PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    segment_id UUID NOT NULL REFERENCES segments(id) ON DELETE CASCADE,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    UNIQUE(user_id, segment_id)
);

CREATE INDEX idx_segment_favorites_user_id ON segment_favorites(user_id);
CREATE INDEX idx_segment_favorites_segment_id ON segment_favorites(segment_id);
