-- Revert: set all weather tag text colors back to white
UPDATE tags
SET text_color = '#ffffff'
WHERE category = 'weather';
