UPDATE users SET email_verified = false WHERE registration_source = 'admin';
UPDATE users SET registration_source = NULL WHERE registration_source = 'admin';
