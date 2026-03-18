ALTER TABLE users ADD COLUMN IF NOT EXISTS noise_settings JSONB
  DEFAULT '{"road":{"enabled":true,"buffer_m":500},"rail":{"enabled":true,"buffer_m":300},"air":{"enabled":true,"buffer_m":3000},"wind":{"enabled":true,"buffer_m":800},"shooting":{"enabled":false,"buffer_m":400},"military":{"enabled":true,"buffer_m":500},"industrial":{"enabled":true,"buffer_m":200}}'::jsonb;
