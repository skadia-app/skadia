UPDATE users SET email_verified = true WHERE legacy_oidc_subject IS NOT NULL;
UPDATE users SET registration_source = 'admin' WHERE legacy_oidc_subject IS NOT NULL;
