-- čas doručenia odoslanej e-faktúry príjemcovi (RFC 3339) podľa poštára
ALTER TABLE einvoices ADD COLUMN delivered TEXT NOT NULL DEFAULT '';
