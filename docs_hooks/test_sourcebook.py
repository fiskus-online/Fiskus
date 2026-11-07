import unittest

from sourcebook import admonitions, cards


class TestSourcebook(unittest.TestCase):
    def test_admonitions(self):
        cases = [
            (':::info[Prečo]\nText\n\nĎalej\n:::\nPo', '!!! info "Prečo"\n    Text\n\n    Ďalej\nPo'),
            (':::tip\nA\n:::', '!!! tip\n    A'),
            ('```md\n:::info[x]\n```', '```md\n:::info[x]\n```'),  # v kóde sa nemení
            (':::warning[Pozor "x"]\nA\n:::', '!!! warning "Pozor \'x\'"\n    A'),
            ('bez bloku', 'bez bloku'),
        ]
        for src, want in cases:
            with self.subTest(src=src):
                self.assertEqual(admonitions(src), want)

    def test_cards(self):
        src = 'Úvod\n```cards\n- title: Predaj\n  href: ./predaj/index.md\n  icon: x\n  text: Faktúry\n- title: Nákup\n```\nKoniec'
        out = cards(src)
        self.assertIn('<div class="grid cards" markdown>', out)
        self.assertIn('-   **[Predaj](./predaj/index.md)**', out)
        self.assertIn('    Faktúry', out)
        self.assertIn('-   **Nákup**', out)
        self.assertTrue(out.startswith('Úvod') and out.endswith('Koniec'))
        self.assertNotIn('```', out)


if __name__ == '__main__':
    unittest.main()
