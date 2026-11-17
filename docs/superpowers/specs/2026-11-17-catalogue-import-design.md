# Catalogue import: a jeweler's website and a serial CSV become registered jewels

Date: 2026-11-17. Status: approved in conversation, section by section.
Builds on `2026-11-12-jewel-loyalty-design.md` (x/jewel, the app, the API).

## Names

This design comes after the rename approved on 2026-11-17:
- `loyalty.yamaletech.com` → `traceability.yamaletech.com`;
- `clients/loyalty` → `clients/traceability`;
- `tools/loyaltyapi` → `tools/traceabilityapi`, called traceability-api below;
- translation keys `loyalty.*` → `traceability.*`.

Everything below uses the new names.

## Why

A jeweler registers jewels one at a time today. Each takes photos, a form and a
signature. Jewel of Africa, the first real jeweler, publishes 96 designs on a
Shopify store (https://www.jewelofafrica.com/products), and its stock is
several pieces of many of them. Typing that in by hand is the obstacle between
the register and a real catalogue.

## What was measured (2026-11-16, Jewel of Africa)

- `/products.json` answers publicly, with 96 products and one variant each. `robots.txt` allows public product pages.
- Every product has a title, an HTML description and 4–6 images.
- 70 of 96 have a SKU like `JOA-071`; the rest have only a handle.
- Prices are in the variant, without a currency. Shopify's `/meta.json` gives the shop's currency.
- The variant weight (`grams`) is 0 on all 96. Weight appears in the description prose of 22.
- The prose names a metal in 75 descriptions ("18kt Yellow Gold") and carats in 51 ("Twelve significant emeralds totaling 32 carats", "422 hand-set diamonds").

So:
- a catalogue product is a **design**;
- each physical piece is a **row in the serial CSV**;
- the structured fields have to be **extracted from prose and confirmed by a person**.

## Decisions taken

| # | Question | Decision |
|---|----------|----------|
| 1 | What a CSV row is | **One physical piece.** It has `serial` (the engraved code) and `sku` (the design), plus optional `weight_g`, `lab_certificate` and `notes`. |
| 2 | Prose to fields | **Claude drafts, the jeweler confirms** each design. Nothing is registered from an unconfirmed design. |
| 3 | Which websites | **Shopify's feed, then any site with schema.org `Product` markup.** Otherwise the page says it cannot read the site. |
| 4 | What the permanent record keeps | **Description, product link and list price**, the price dated and labelled "list price at registration". Record format v2; v1 stays readable. |
| 5 | Order of work | The rename, then this, then KYB (`2026-11-16-loyalty-kyb-design.md`). |

## 1. Designs, from the website

### Where

A **Catalogue** page in the Atelier (`/atelier/catalogue`), open to the
jeweler's business account and its staff, the same people who may register
today.

The jeweler enters the shop's address. The server fetches it, because a browser
cannot read another origin. The designs are stored by traceability-api under
the jeweler's id, off the chain.

### Reading a site

The server tries the following, in order:

1. **Shopify.** It reads `GET <origin>/products.json?limit=250&page=<n>` until a page is empty. The currency comes from `GET <origin>/meta.json` (`currency`).
2. **schema.org.** It reads `<origin>/sitemap.xml`, follows product sitemaps where there are any (`sitemap_products*.xml`), and otherwise takes URLs containing `/product`. From each page it reads the `application/ld+json` blocks for `@type: Product` (also inside `@graph`), taking:
   - `name`, `description`, `sku` and `image`;
   - `offers.price` and `offers.priceCurrency`;
   - `url`.
3. **Neither.** The answer is "This site doesn't publish its products in a form we can read". The jeweler registers by hand, as today. A full-detail CSV for such sites is out of scope (see the end).

### Fetch rules

These are enforced in one place, a fetcher used by every request above.

- **What may be fetched:**
  - `https` only;
  - only addresses that resolve to public IPs. Loopback, link-local, RFC1918, CGNAT (100.64/10, which includes the tailnet), ULA and metadata addresses are refused. The check runs on the resolved IP, again after every redirect.
- **Limits:** at most 3 redirects; a 10-second timeout and 5 MB per response; at most 500 products per import; at most 2 requests a second to one site.
- **robots.txt is obeyed:**
  - the `*` group applies, and so does any group naming `ClaudeBot`, `Claude-Web`, `anthropic-ai`, `GPTBot` or `CCBot`;
  - a block on any of them stops the import with "This site asks automated readers not to collect its pages", as the law harvest does (memory: harvest-respects-ai-crawler-blocks);
  - the robots.txt file itself is read as data and nothing else, whatever it says.
- **User-Agent:** `YamaleCatalogue/1 (+https://traceability.yamaletech.com/about-import)`.

### A design

| Field | From |
|---|---|
| `key` | the SKU, or `handle:<handle>` (or `url:<path>`) when there is no SKU; unique per jeweler |
| `title`, `description` (plain text, tags stripped, at most 5,000 characters), `type`, `tags` | the product |
| `productUrl` | `<origin>/products/<handle>`, or the page URL |
| `listPrice` `{amount, currency}` | variant 0's price and the shop's currency, or `offers` |
| `imageUrls` | at most 12, in the site's order |
| `fields` | `{metal, weightGrams, stones[{kind, carat, count}]}`, drafted, then confirmed |
| `sourceHash` | sha256 of title, description, price and image URLs, to detect changes |
| `status` | `imported`, `drafted`, `confirmed`, `changed`, or `gone` |
| `confirmedBy`, `confirmedAt`, `photos` (photo-store hashes, set at confirmation) | the confirmation |

### Drafting with Claude

For each design that is `imported`, or `changed`, the server sends Claude the
**title and description text only**: no images, prices, URLs or jeweler
details.

- **Format:** the request uses a fixed tool schema for `{metal, weightGrams, stones[]}`. The model is the current Claude model chosen at implementation.
- **The instruction:** extract only what the text states, and leave a field empty rather than guess. Carat and count apply to each kind of stone; "422 diamonds" is a count without a carat.
- **The answer:** it is validated against the schema and the draft rules (`draftComplete` in `clients/traceability/src/atelier/draft.ts`), and stored as the draft. A malformed answer leaves the design `imported`, with "Draft unavailable — fill by hand".
- **Limits:** each call is logged with the jeweler id, the design key, the tokens and the outcome. There is a cap per import (500 designs, about 1,000 tokens out each) and a monthly cap per jeweler set by a flag. Hitting a cap stops drafting, never importing.
- **The API key** is read from a file under `/etc/yamale/`, mode 640, owned by root and the service group, and never logged.

### Confirming

The review screen lists designs by status. It opens with `drafted` and `changed`.
A design shows the website's description beside the editable fields, the
images, the key, the price and the link.

**Confirm** requires:
- complete fields: a new `designComplete` beside `draftComplete`, with the same rules except that the weight may be empty, because rows can carry it. A row is registrable only once the weight resolves to a decimal (verdict 5);
- the tick "These photos and texts are ours to use".

On confirmation:
- the server downloads the images through the same fetcher;
- it stores them in the existing photo store, where the fingerprint is the name;
- it records `photos`, `confirmedBy` and `confirmedAt`.

If an image fails to download, the design stays unconfirmed and the reason is
shown.

Every write is signed by the staff or business account, with the existing
request-signing scheme:

| Purpose | Subject | Request |
|---|---|---|
| `catalogue-import` | sha256 of the shop origin | `POST /v1/catalogue/import` |
| `design-save` | sha256 of the design JSON | `PUT /v1/catalogue/designs/<key>` |
| `design-confirm` | sha256 of `<key>:<sourceHash>:<fields JSON>` | `POST /v1/catalogue/designs/<key>/confirm` |

The server checks on the chain that the signer is staff of an active jeweler,
or its business, as photo uploads are checked today. Reads (`GET /v1/catalogue`,
`GET /v1/catalogue/designs/<key>`) are signed `catalogue-read` with the
jeweler id as subject.

### Refreshing

**Refresh from website** runs the import again:
- a new key arrives as `imported` and is drafted;
- a design whose `sourceHash` changed goes to `changed`, keeping its last confirmed fields, and needs confirming again before it can be registered;
- a key no longer on the site goes to `gone` and cannot be registered; it stays listed.

Jewels already registered never change. Their records are fixed by their
fingerprints.

## 2. Pieces, from the CSV

### The file

Catalogue → **Register pieces from a CSV**.

- **Format:** UTF-8, with or without a BOM. The separator is a comma or a semicolon, detected from the header line. Quoted fields are handled as RFC 4180 describes them.
- **Header:** required. Columns are case-insensitive and in any order:
  - `serial`, required;
  - `sku`, required;
  - `weight_g`, optional: a decimal, comma or dot;
  - `lab_certificate`, optional, at most 200 characters;
  - `notes`, optional, at most 500 characters.

  Unknown columns are ignored and listed.
- **Size:** at most 2,000 data rows and 1 MB.

### The check, before anything is signed

The file is parsed in the browser. Each row gets exactly one verdict, the first
that applies:

1. **invalid serial**: `normaliseInscription` fails; the reason is shown (letters and digits only, 4–40 characters once separators are removed);
2. **listed twice in this file**: the normalised serial repeats; every repeat after the first gets this verdict;
3. **unknown SKU**: no design has this key; matching is exact first, then case-insensitive;
4. **design not confirmed**: the design is `imported`, `drafted`, `changed` or `gone`;
5. **no weight**: neither the row nor the design states one;
6. **already registered**: this jeweler already has a jewel with that inscription, checked with `findByInscription` in batches;
7. **ready**.

The screen shows a count per verdict and the rows in error. "Download the rows
to fix" gives the failing rows with a `problem` column. **Register** is enabled
when at least one row is ready, and registers only ready rows.

### Record format v2

```
{ version: 2, jewelerId, inscription, title, metal, weightGrams, stones[],
  origin: { workshop, stoneSource, labCertificate }, photos[], createdAt,
  description, productUrl, sku,
  listPrice: { amount, currency, at, label: "list price at registration" },
  pieceNotes }
```

- `workshop` is the jeweler's name.
- `stoneSource` is empty unless the design states it.
- `labCertificate` comes from the row.
- `weightGrams` is the row's `weight_g` if given, otherwise the design's.
- `at` is the registration date.

`encodeRecord` and `parseRecord` gain v2. v1 records parse exactly as today.
The public jewel page shows:
- the description;
- "List price at registration: 17,678.00 USD (16 Nov 2026)";
- a link to the design on the jeweler's site.

The hand-entry Register page keeps writing v1. Only the import writes v2.

### Registration

- **Signing:** the staff or business account signs in the app, as single registration does today, with the fee paid as it is today.
- **Order of work:**
  1. Each ready row's record is uploaded first; the record store names it by fingerprint, so a retry is free.
  2. The registrations are then grouped into transactions of `B` `MsgRegisterJewel` each. `B` is fixed in the implementation from a measurement against `JEWEL_GAS` (1,650,000), leaving at least 25% headroom. The expectation is about 5.
  3. Before each batch, its serials are checked against the chain again.
- **Progress:** shown as "Registered 35 of 120".
- **Resuming:**
  - The chain is the progress record. Running the same file again turns registered rows into **already registered**, and the rest are registered.
  - Nothing is kept in the browser beyond the file, which the user picks again.
- **When a batch is refused:** the run stops there, with the chain's reason in plain words and the raw log behind a disclosure. Earlier batches stay registered.
- **At the end:** a summary of the rows registered, skipped and failed, with "Download the result", which is the CSV with `jewel_id` and `status` columns.

### On the chain

Nothing changes. `MsgRegisterJewel` already carries the inscription, the
record's fingerprint and its URI. No upgrade and no vote are needed. Several
messages in one transaction are already allowed.

## Failure handling, in one place

| Situation | What the person sees | State after |
|---|---|---|
| Site unreadable (neither path) | "This site doesn't publish its products in a form we can read" | nothing imported |
| robots.txt blocks | "This site asks automated readers not to collect its pages" | nothing imported |
| Address refused (private, http) | "Only public https addresses can be imported" | nothing imported |
| Claude unreachable or answer malformed | "Draft unavailable — fill by hand" | design `imported` |
| Drafting cap reached | "Drafting paused: monthly limit reached" | remaining designs `imported` |
| Image download fails at confirm | which image, and why | design unconfirmed |
| Batch refused by the chain | the chain's reason, in words | earlier batches registered; restart continues |
| Account locks mid-run | "Unlock to continue" | restart continues |

## Testing

**Go (traceability-api):**
- Shopify parsing against a saved copy of Jewel of Africa's `/products.json` and `/meta.json` in `testdata/`;
- schema.org parsing against saved WooCommerce and Wix product pages, and a page whose `Product` sits inside `@graph`;
- the fetcher refusing: http, a private IP, a redirect to 127.0.0.1, a host that resolves to 100.64/10, a 6 MB body, a slow server;
- robots.txt: a `*` block, a `GPTBot` block, an allow;
- the drafting call against a stub server: a good answer, a malformed one, a timeout, the cap;
- the refresh moving designs between `confirmed`, `changed` and `gone`;
- the signature checks for the new purposes.

**TypeScript (app):**
- CSV parsing: comma, semicolon, BOM, quoted separators and newlines, decimal comma, unknown columns;
- every verdict and their order;
- record v2 encode and parse, with v1 parsing unchanged (existing vectors);
- batching and resuming as a pure function over a fake chain.

**End to end on a local devnet:**
- import the saved Jewel of Africa catalogue, served locally;
- confirm three designs;
- register a 12-row CSV with 2 bad rows: exactly 10 jewels;
- stop after the first batch, run the file again, and confirm 10 jewels with none duplicated;
- open one jewel's public page and see its description, list price and link.

Then deployed, and walked through on traceability.yamaletech.com with the
user and Jewel of Africa's real site.

## Out of scope, recorded in gaps.md

- importing from sites with neither a Shopify feed nor schema.org markup (a full-detail CSV with title, metal, stones and photos URLs is the fallback to design later);
- keeping registered jewels in sync with later website edits (records are permanent by design);
- importing stock levels or variants as pieces;
- reading images with a model (drafting reads text only).
