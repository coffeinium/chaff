UPDATE indicators SET note = threat WHERE note = '' AND threat <> '';
ALTER TABLE indicators DROP COLUMN threat;
