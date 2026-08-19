# Languages

Covering "most of the 54" is not 54 translations. Four languages reach the
official language of about fifty countries; the work that actually matters is
the second tier, where people count money in the language they think in.

## Three tiers

**Tier 1 — pivot languages.** English, French, Arabic, Portuguese. Between them
these are official or co-official in roughly fifty of the fifty-four member
states. Everything ships in all four or it does not ship. Swahili belongs here
in practice rather than by status: it is a working lingua franca across the East
African Community and reaches more daily speakers than Portuguese does.

**Tier 2 — the languages people actually transact in.** Ordered by speakers, not
by state:

    Hausa       Nigeria, Niger, Ghana, Chad          ~80M
    Amharic     Ethiopia                             ~35M
    Oromo       Ethiopia, Kenya                      ~37M
    Yoruba      Nigeria, Benin, Togo                 ~45M
    Igbo        Nigeria                              ~30M
    Zulu        South Africa                         ~28M
    Somali      Somalia, Ethiopia, Kenya, Djibouti   ~22M
    Fula        Sahel, fifteen countries             ~35M
    Akan/Twi    Ghana                                ~20M
    Shona       Zimbabwe                             ~15M
    Malagasy    Madagascar                           ~25M
    Kinyarwanda Rwanda, Burundi                      ~20M
    Wolof       Senegal, Gambia, Mauritania          ~12M
    Bambara     Mali                                 ~15M
    Xhosa       South Africa                         ~11M

**Tier 3 — everything else.** Real, but reached through community contribution
rather than commissioned translation. The system has to make that possible from
day one or it never happens.

Note what tier 2 does that tier 1 cannot: Fula follows the Sahel across fifteen
borders, and Hausa crosses four. These are trade languages. A cross-border
payments network that speaks only the coloniser's language at the border post
has missed who is actually moving money.

## The hard parts are not translation

**Arabic is right-to-left.** This is a layout problem, and it is the single
largest piece of engineering here. Every flex direction, every icon that means
"forward", every transaction row with an amount on one side and a name on the
other has to mirror. CSS logical properties (`margin-inline-start`, not
`margin-left`) make this nearly free if adopted before the UI is built and
extremely expensive afterwards. **Adopt them now, while there are four
frontends and not forty screens.**

**Amharic and Tigrinya use Ge'ez script; Tifinagh and N'Ko are not Latin
either.** These fonts are absent from many Android devices in the field. Either
subset and self-host the fonts or the user sees boxes — and a payment screen
full of boxes is worse than English.

**Text expands.** French runs 15–25% longer than English, German more, and
compound-forming languages worse still. Any button sized to its English label
breaks. Test the layout in the longest language, not the shortest.

**Arabic has six plural forms**; most Bantu languages have noun-class agreement
that plural rules do not model at all. Use ICU MessageFormat and let the
catalogue carry the rule, rather than concatenating a count with a noun in code.

**Numbers and money are locale data, not string data.** Decimal separators,
digit grouping, and currency symbol placement all vary — and Arabic may use
Eastern Arabic numerals. Format through `Intl.NumberFormat` with the currency
and locale, never by string manipulation.

## Never translate the chain's errors

The chain returns errors in English from Go, and they are not user-facing text —
they are diagnostics that happen to be in words. Translating them means shipping
a translation pipeline that runs at the speed of chain upgrades and produces
sentences no user asked for.

Instead: **the chain returns stable error codes; the client owns the message.**
`ErrDuplicateRef` becomes a catalogue key, and the catalogue says, in Wolof,
what the user should do about it. This also means an error's wording can be
fixed without a consensus-breaking upgrade, which is the same reasoning that
keeps search and presentation out of consensus.

This has a prerequisite worth naming: every module's errors need stable,
enumerated codes that clients can switch on. That is worth auditing before
translation starts, not after.

## Money words are not ordinary words

"Balance", "pending", "fee", "settled", "frozen", "available" carry meaning a
central bank will hold you to. A volunteer translator rendering "pending" as
something closer to "maybe" produces a legally different product.

So the glossary is a deliverable, not a by-product: a fixed list of financial
terms with a definition in English, translated once by someone accountable, and
locked. Community contribution applies to the rest of the interface, not to
these. `x/enforcement` freezing an account and the word for "frozen" have to
mean the same thing in every language, because someone will read it in court.

## Architecture

One catalogue, shared by every surface. The four frontends, the site, and the
docs all consume the same keys from the SDK — a term translated once is
translated everywhere, and a term that drifts is visible immediately.

- **ICU MessageFormat**, for plural and gender rules the catalogue can express
  and code cannot.
- **Locale resolution**: explicit user choice, then device locale, then the
  country's tier 1 language, then English. Never infer from IP.
- **Lazy-load catalogues.** Fifty locales in one bundle is a mobile data bill in
  a region where data is the constraint.
- **Language is a device preference, not account state.** It does not belong
  on-chain: it is not consensus, and putting it there makes changing your
  interface language a transaction that costs money.

## Sequencing

1. **CSS logical properties and an RTL audit**, before more UI is built. This is
   the decision that gets expensive with delay.
2. **Extract strings to a catalogue** with ICU, English as source.
3. **Stable error codes** across every module, replacing English strings at the
   client boundary.
4. **Financial glossary**, locked, translated accountably.
5. **Tier 1** — French, Arabic, Portuguese, Swahili.
6. **Community contribution**, with the glossary enforced and the tier 2 list as
   the target.

Step 1 is the only one that is harder tomorrow than today.
