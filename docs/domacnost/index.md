---
title: Domácnosť
sidebar_position: 9
description: Domáce účtovníctvo — účty, výdavky a príjmy podľa kategórií, mesačný rozpočet a peniaze zo živnosti.
tags: [domácnosť, rozpočet]
---

# Domácnosť

Domáce účtovníctvo vediete v samostatnej **domácnosti** — vo Fiškuse je to
zvláštny druh „firmy“ bez IČO, DPH a dokladov. Založíte ju v časti
**Nastavenia → Firmy a domácnosti → Založiť domácnosť**. Do počtu firiem
licencie sa nepočíta, takže ju má aj bezplatná desktopová verzia popri
svojej firme. Medzi firmou a domácnosťou sa prepínate hore v hlavičke.

## Účty

Účet je miesto, kde máte peniaze: bežný účet, peňaženka, kreditná karta,
sporenie. Pri účte zadáte **počiatočný stav** — zostatok v deň, od ktorého
zapisujete pohyby. Ďalej ho Fiškus počíta sám. Účet s pohybmi sa nedá
zmazať; keď ho zrušíte v banke, označte ho ako zrušený.

## Pohyby

| Druh | Čo to je |
|---|---|
| Výdavok | nákup, platba, splátka — znižuje zostatok účtu |
| Príjem | mzda, peniaze zo živnosti, dávky — zvyšuje zostatok |
| Prevod | výber z bankomatu, presun na sporenie — do príjmov ani výdavkov sa nepočíta |

Kategóriu navrhne Fiškus podľa obchodu: najprv podľa toho, kam ste rovnaký
obchod zaradili naposledy, potom podľa známych obchodov (Lidl → Potraviny,
Slovnaft → Auto a doprava, ZSE → Energie…).

**Výpis z banky** (camt.053, GPC, CSV, Excel) nahráte tlačidlom *Nahrať
výpis*. Kategórie sa navrhnú rovnako; opakované nahratie toho istého výpisu
nič nezdvojí. Platby v cudzej mene sa vynechajú.

## Bločky

Tlačidlo **Bloček** (v prehľade aj v pohyboch) otvorí fotoaparát alebo výber
fotky. Bloček odfoťte celý, aj s QR kódom.

| Bloček | Odkiaľ sú položky |
|---|---|
| s QR kódom eKasa | priamo z Finančnej správy — presné názvy a ceny |
| bez QR kódu | Fiškus ho prečíta z fotky cez Claude API (kľúč v Nastaveniach → Prístup pre AI) |

Každú položku Fiškus zaradí do kategórie: rožky a mlieko do potravín,
šampón do drogérie, lieky do zdravia. Pred zapísaním ich skontrolujete
a zmeníte; tlačidlom *Všetky do kategórie…* zaradíte celý nákup naraz.
Ako položku zaradíte, tak ju Fiškus navrhne nabudúce. Zľava je samostatná
položka, rozdiel k celkovej sume (zaokrúhlenie) tiež.

Bloček sa zapíše ako jeden výdavok rozdelený na položky; prehľad
a rozpočet počítajú každú kategóriu zvlášť. Fotka ostane pri pohybe.
Ten istý bloček sa druhýkrát nezapíše.

Na telefóne spárovanom s programom (*Mobil v sieti*) sa bloček po odfotení
zapíše hneď s navrhnutými kategóriami, upraviť ich môžete neskôr v Pohyboch.

## Peniaze zo živnosti

Tlačidlo *Zo živnosti* ukáže výbery a vklady podnikateľa z peňažného denníka
vašej živnosti — pohyby zaradené do stĺpcov *mimo základu dane*. Výber sa
v domácnosti zapíše ako príjem, vklad do živnosti ako výdavok. Prenesený
pohyb sa druhýkrát neprenesie.

## Záväzky a platby

Sem patria pôžičky a úvery, dlhy, exekúcie, pravidelné platby (nájom,
zálohy na energie, poistky) aj peniaze, ktoré ste niekomu požičali.

| Údaj | Na čo slúži |
|---|---|
| Dlžná suma | celkom, pri exekúcii vrátane trov; pri pravidelnej platbe sa nezadáva |
| Splatené skôr | čo ste zaplatili pred evidenciou vo Fiškuse |
| Splátka, opakovanie, prvá splátka | z nich Fiškus zostaví splátkový kalendár |
| Počet splátok | 0 = kým sa dlh nesplatí (pri pravidelnej platbe bez konca) |
| Kategória a účet | predvyplnia sa pri zaplatení |

Tlačidlom **Zaplatiť** zapíšete splátku ako výdavok (pri požičaných peniazoch
**Prijať** ako príjem). Splátku môžete priradiť aj pri bežnom zápise pohybu
v poli *Splátka záväzku*. Zaplatené sa započíta do splátok v poradí
splatnosti; nezaplatená splátka po termíne je červená.

Splátky sú v **Kalendári** a v prehľade domácnosti (najbližší mesiac
a všetko po splatnosti). Ak máte v **Nastaveniach → E-mail** zapnuté
upozornenia na lehoty, Fiškus pripomenie splátky e-mailom vopred.

## Rozpočet

V časti **Kategórie a rozpočet** zadáte pri kategóriách výdavkov sumu na
mesiac. Prehľad ukáže, koľko z rozpočtu v mesiaci ostáva; prekročené
kategórie sú červené. Kategórie si môžete premenovať, pridať aj odobrať.
