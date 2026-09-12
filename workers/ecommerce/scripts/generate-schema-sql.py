#!/usr/bin/env python3
"""Rewrites functions/api/shop/schema.sql from the migrations.

The migrations in _schema.ts are the source of truth; schema.sql is the same
thing laid out for someone who wants to read the tables without reading
TypeScript, or to create a local database in one command. Run this after adding
a migration — a test compares the two and fails when they disagree.

    python3 scripts/generate-schema-sql.py
"""

import re

src = open('functions/api/shop/_schema.ts').read()
stmts = [re.search(r'const BOOKKEEPING = `([^`]+)`', src).group(1)]
start = src.index('const MIGRATIONS')
end = src.index('/** The EU + UK standard')
stmts += re.findall(r'`([^`]+)`', src[start:end])

header = """-- Schema for the ecommerce worker, written out for a human.
--
-- You do not have to run this: the worker applies its own migrations on first
-- use (see _schema.ts). It is here so the tables can be read without reading
-- TypeScript, and so a local database can be created in one command:
--
--   wrangler d1 execute shop --local --file=functions/api/shop/schema.sql
--
-- A test compares this file's objects against the ones the migrations actually
-- create, so the two cannot drift apart unnoticed.
"""

blocks = []
for stmt in stmts:
    lines = stmt.strip().split("\n")
    fixed = [lines[0].strip()]
    for line in lines[1:]:
        fixed.append("  " + line.strip() if line.strip() else "")
    body = "\n".join(fixed)
    if body.endswith("  )"):
        body = body[:-3] + ")"
    blocks.append(body + ";")

open('functions/api/shop/schema.sql', 'w').write(header + "\n" + "\n\n".join(blocks) + "\n")
print(len(blocks), "statements")
