"""Preklad dialektu Sourcebook do MkDocs Material pri zostavení návodu.

Zdrojové stránky v docs/ ostávajú v dialekte Sourcebook (berie si ich aj
platforma fabrikaERP). Pre docs.fiskus.online sa pri zostavení preložia:

- ``:::info[Nadpis]`` … ``:::`` → admonícia ``!!! info "Nadpis"``
- blok ````cards`` (YAML zoznam title/href/text) → mriežka kariet Material
"""

import re

_OPEN = re.compile(r'^:::(\w+)(?:\[(.*)\])?\s*$')
_FENCE = re.compile(r'^(```|~~~)')


def admonitions(md: str) -> str:
    """:::typ[Nadpis] … ::: → !!! typ "Nadpis" s odsadeným obsahom."""
    out, body, head = [], None, None
    fence = False
    for line in md.split('\n'):
        if body is None:
            if _FENCE.match(line):
                fence = not fence
            m = None if fence else _OPEN.match(line)
            if m:
                kind, title = m.group(1), m.group(2)
                head = f'!!! {kind}' + (' "' + title.replace('"', "'") + '"' if title else '')
                body = []
            else:
                out.append(line)
            continue
        if line.strip() == ':::':
            out.append(head)
            out.extend(('    ' + b) if b.strip() else '' for b in body)
            body = None
        else:
            body.append(line)
    if body is not None:  # neuzavretý blok — nechať ako text
        out.append(head)
        out.extend(body)
    return '\n'.join(out)


def _cards(items: list) -> str:
    rows = ['<div class="grid cards" markdown>', '']
    for it in items:
        title, href, text = it.get('title', ''), it.get('href', ''), it.get('text', '')
        link = f'[{title}]({href})' if href else title
        rows.append(f'-   **{link}**')
        if text:
            rows += ['', '    ' + text]
        rows.append('')
    rows.append('</div>')
    return '\n'.join(rows)


def cards(md: str) -> str:
    """```cards so zoznamom - title/href/icon/text → mriežka kariet."""
    out, items, inside = [], None, False
    for line in md.split('\n'):
        if not inside:
            if line.strip() == '```cards':
                inside, items = True, []
            else:
                out.append(line)
            continue
        if line.strip() == '```':
            out.append(_cards(items))
            inside = False
            continue
        m = re.match(r'^(-\s+|\s+)(\w+):\s*(.*)$', line)
        if not m:
            continue
        if m.group(1).startswith('-'):
            items.append({})
        if items:
            items[-1][m.group(2)] = m.group(3).strip()
    return '\n'.join(out)


def translate(md: str) -> str:
    return admonitions(cards(md))


def on_page_markdown(markdown, **kwargs):  # háčik MkDocs
    return translate(markdown)
