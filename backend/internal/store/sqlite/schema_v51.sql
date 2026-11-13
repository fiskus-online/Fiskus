-- ďalšie bankové účty kontaktu (JSON: iban, účel)
ALTER TABLE contacts ADD COLUMN accounts_json TEXT NOT NULL DEFAULT '';
