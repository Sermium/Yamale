# The presentation site

One page answering "what is this for, and why should I care?" for somebody who
has not heard of Yamale and may not care about blockchains at all.

## Running it

```bash
python -m http.server 4173
```

**From the repository root, not from this directory.** The page links to
`../../docs/…`, which resolves correctly when the server's root is the repo and
404s when it is `clients/site` — `..` clamps at the document root, so those
links silently become `/docs/…` with nothing behind them. Then open
<http://localhost:4173/clients/site/index.html>.

## Why it is a static page

No framework, no build step, no dependencies. This is the one surface a stranger
loads first, often on a phone, often on a bad connection — and it is a page of
text. Anything that needs `npm install` to render a paragraph is a liability
here, and it would also make the site the only part of the project that could
break without a single line of it changing.

It shares the explorer's design tokens so the two feel like one product, but
carries its own copy of them rather than importing: the explorer's stylesheet
comes with an application's worth of component CSS this page has no use for.

## When editing

**Lead with the use case.** Nobody outside the industry has ever been persuaded
by a consensus algorithm. The words "Tendermint", "BFT" and "IBC" appear nowhere
on this page, deliberately — the people who want them will find them in the
[reference](../../docs/reference/).

**Keep the status section honest.** It says what is not built and not audited.
That is more persuasive to institutional readers than implied readiness a first
conversation would puncture, and it is the section most likely to go stale —
check it against [the docs' own missing list](../../docs/README.md) when the
project moves.

**Both themes, every viewport.** Light and dark are driven by
`prefers-color-scheme`, and the layout has been checked at 375px and 1280px with
no horizontal overflow at either. Status marks carry a word and a shape, not
only a colour.
