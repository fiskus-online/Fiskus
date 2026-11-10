-- prijaté objednávky od zákazníkov: odberateľ, položky, dodania (JSON)
CREATE TABLE sales_orders (
    id         INTEGER PRIMARY KEY,
    company_id INTEGER NOT NULL REFERENCES companies(id),
    number     TEXT NOT NULL DEFAULT '',
    data       TEXT NOT NULL
);
CREATE UNIQUE INDEX sales_orders_number ON sales_orders (company_id, number) WHERE number <> '';
