# Jeweler KYB: apply in the app, admitted by an admin's click

Names updated 2026-11-17 for the rename to traceability.

Date: 2026-11-16. Status: approved in conversation, section by section.
Builds on `2026-11-12-jewel-loyalty-design.md` (x/jewel, the traceability app,
traceability-api) as deployed on devnet-2 on 2026-11-16 (then at loyalty.yamaletech.com, now traceability.yamaletech.com).

## Why

A business can already apply from the app (Account → For jewelers → Apply,
route `/apply`). The application is signed with the account that will become
the business, and traceability-api spools it on yamale-us. Nothing after that is
usable by a person:

- there is no evidence to check;
- the application can only be read with the operator token, from the server;
- the applicant never hears back;
- admission is `jewel-admit` run over ssh by someone with sudo.

This design turns that into a product. A business applies with documents. An
admin reviews them in the app and approves with one click, and the office's
decision executes on the chain without anyone opening a terminal.

## Decisions taken

| # | Question | Decision |
|---|----------|----------|
| 1 | Who approves, and what approval means on the chain | **One admin, one click.** The server casts both of the office's votes (2-of-3 x/group 10). The admin's login is therefore the security of admission. Decision 5 is the mitigation. |
| 2 | KYB evidence | **Uploaded documents.** A registration extract and the director's identity document are required; up to three more files are optional. Stored encrypted on yamale-us, never on the chain. |
| 3 | Where the admin works | **An Admin tab in the traceability app**, shown to allowlisted accounts. Every admin request is signed with the admin's own account key. There are no passwords. |
| 4 | Notifications | **In-app status plus email.** The applicant is emailed at each decision; the admin is emailed on each submission. Mail goes through the Mailcow server on yamale-us. |
| 5 | What holds the office keys | **A separate office signer** (`yamale-jewel-office`), listening on localhost only. It re-verifies the admin's signed request itself, so the internet-facing traceability-api never holds an office key. |
| 6 | Admin scope | Decide applications, **and suspend or reinstate** admitted jewelers. |

## 1. The applicant

### States

One application per business account (the signing address).

| State | Meaning | The applicant can |
|---|---|---|
| `draft` | started, not submitted | edit fields, upload, delete files, submit |
| `submitted` | waiting for an admin | read only |
| `more_needed` | an admin asked for more; the note is shown | edit, upload, resubmit, which goes back to `submitted` |
| `admitted` | the office admitted it on the chain; the jeweler id is shown | open the Atelier |
| `refused` | an admin refused; the reason is shown | read; apply again 30 days after the refusal |
| `abandoned` | a `draft` or `more_needed` left 90 days without activity; documents deleted (see Retention) | start a new application at once |

Transitions:
- `draft → submitted`
- `submitted → more_needed | admitted | refused`
- `more_needed → submitted`
- `refused → draft`, only by the applicant and after 30 days, as a new application (the old one is kept)
- `draft | more_needed → abandoned`, by the nightly retention job only
- `admitted` and `abandoned` are final

### Fields

These are today's fields, kept with today's rules (`tools/traceabilityapi/applications.go` `check()`):

| Field | Rule |
|---|---|
| business name | 2–120 |
| contact | 2–120 |
| email | required, at most 254 |
| phone | optional, at most 40 |
| country | ISO-2, in capitals |
| message | at most 2,000 |

These are added:

| Field | Rule |
|---|---|
| registration number | 2–60 |
| legal form | 2–60, free text |
| registered address | 5–300 |
| director's full name | 2–120 |

### Documents

| Kind | Required |
|---|---|
| `registration` (company registration extract) | yes, exactly one |
| `director_id` (the director's identity document) | yes, exactly one |
| `other` | optional, at most three |

Rules:
- **Types:** PDF, JPEG or PNG. The type is decided by the file's first bytes, not by its name or its `Content-Type`.
- **Size:** at most 10 MB a file and 25 MB an application.
- **Metadata:** the app removes EXIF from JPEGs before upload by re-encoding through a canvas, and PNG metadata the same way. PDFs are sent as they are.
- **Submitting:** the form refuses to submit until both required documents are present.

### Signing

Every applicant write is signed by the business account, with the existing
scheme in `tools/traceabilityapi/auth.go` and `clients/sdk/src/traceabilityauth.ts`:

```
yamale-traceability/v1:<purpose>:<chain id>:<subject>:<issued>
```

| Purpose | Subject | Request |
|---|---|---|
| `application` | sha256 hex of the JSON body | `PUT /v1/applications/mine` (create or replace the draft fields) |
| `kyb-file` | sha256 hex of `<kind>:<file sha256 hex>` | `POST /v1/applications/mine/files?kind=<kind>` (body = the file) |
| `kyb-file-delete` | `<file id>` | `DELETE /v1/applications/mine/files/<file id>` |
| `application-submit` | `<application id>` | `POST /v1/applications/mine/submit` |
| `application-read` | `<signer address>` | `GET /v1/applications/mine` (the applicant's own status, fields, file list and the admin's note or reason; never another account's) |

The existing five-minute window applies, and so does the reason it is safe:
each of these requests is idempotent for the same body.

The old `POST /v1/applications` is kept for one release and answers by creating
and submitting in one step without files. A submission that arrives through it
behaves like a migrated application (below).

### In the app

The Account tab's For jewelers section becomes one of:
- "Apply as a jeweler", when there is no application;
- "Continue your application", in `draft` or `more_needed`;
- the status line, in `submitted`, `admitted` or `refused`, with the note or reason when there is one.

`/apply` becomes a single page:
- the fields;
- two required upload slots and an optional list;
- a submit button, disabled with its reason until the application is complete.

Its text is in all seven of the app's languages, like the rest of it.

### Email to the applicant

The application's email address receives a message:
- when the application is received;
- when more is needed, quoting the admin's note;
- when it is admitted, with the jeweler's public name;
- when it is refused, quoting the reason.

The messages carry no attachment and no link that signs in or acts. They name
the app's address and tell the reader to open it.

### Migration

Spooled applications from before this change load as `submitted`, with no
files. The review screen marks them "no documents (applied before KYB)", so the
admin can ask for more.

## 2. The admin, and the office signer

### Who is an admin

`/etc/yamale/traceability-admins` on yamale-us. Each line is
`<yml address> <display name…>`; `#` starts a comment.
- traceability-api and the office signer both read it at start and on SIGHUP.
- The installer creates it empty, owned by root and readable by both services' groups (640).
- Adding an admin is an edit and a reload, never an in-app action.

### Endpoints

All admin endpoints are under `/v1/admin/`. Each answers **404** to any request
that lacks a valid signature from a listed admin, so an outsider cannot tell
they exist.

| Request | Purpose | Subject |
|---|---|---|
| `GET /v1/admin/whoami` | `admin-read` | `whoami` |
| `GET /v1/admin/applications?state=…` | `admin-read` | `applications:<state>` |
| `GET /v1/admin/applications/<id>` | `admin-read` | `application:<id>` |
| `GET /v1/admin/applications/<id>/files/<file id>` | `admin-read` | `file:<file id>` |
| `GET /v1/admin/jewelers` | `admin-read` | `jewelers` |
| `POST /v1/admin/applications/<id>/approve` | `office-admit` | sha256 of the canonical action JSON |
| `POST /v1/admin/applications/<id>/more` | `admin-more` | sha256 of the canonical action JSON |
| `POST /v1/admin/applications/<id>/refuse` | `admin-refuse` | sha256 of the canonical action JSON |
| `POST /v1/admin/jewelers/<id>/suspend` | `office-suspend` | sha256 of the canonical action JSON |
| `POST /v1/admin/jewelers/<id>/reinstate` | `office-reinstate` | sha256 of the canonical action JSON |

`whoami` answers the admin's display name. The app shows the Admin tab only
when that call succeeds. Hiding the tab is a convenience, not the control.

The canonical action JSON has sorted keys and no whitespace:

```json
{"action":"admit","application":"<id>","business":"<yml…>","name":"<public jeweler name>","checked":true}
{"action":"more","application":"<id>","note":"<text>"}
{"action":"refuse","application":"<id>","reason":"<text>"}
{"action":"suspend","jeweler":"<id>","reason":"<text>"}
{"action":"reinstate","jeweler":"<id>"}
```

Admin actions are stricter than the applicant's:
- **60-second window**, not five minutes.
- **No replay.** The verifier keeps every accepted signature until its window has passed, and refuses a second use.
- **Admit requires `checked: true`** (the "I checked the documents" box) and a name of 2–120 characters, defaulting in the app to the business name.
- **Notes and reasons** are 5–1,000 characters.

### The review screen

The Admin tab has two lists.

**Applications**, `submitted` first and oldest first, with the count on the tab.
An application shows:
- every field;
- each document, opened inline in the browser;
- the history: every state change, who made it, when, and the note.

The actions are:
- **Approve**: the box must be ticked and the public name confirmed.
- **Ask for more**: needs a note.
- **Refuse**: needs a reason.

Before an action is signed, the screen states its effect in plain words:
- Approve: "Maison Diop's account becomes a jeweler on the chain; it can register jewels immediately."
- Suspend: "Stops this jeweler's stock sales and new registrations; jewels already sold are unaffected."

**Jewelers**: everything `query jewel jewelers` returns, with the status.
**Suspend** needs a reason; **Reinstate** does not.

### Admission and suspension: the office signer

The signer is `tools/jewelofficed`, a new binary run as `yamale-jewel-office.service`.
- It runs under its own user, `yamale-jewel-office`, and listens on 127.0.0.1:8097.
- It owns `/opt/yamale/jewel-office`, the keyring that holds `jewel-office-1..3` and nothing else, moved from root to that user at install.
- It reads the admin list.
- It has no route through the gate and none through Caddy.

It has three endpoints: `POST /admit`, `POST /suspend` and `POST /reinstate`.
traceability-api forwards to them the admin's **original** request:
- the three signature headers;
- the canonical action JSON as the body;
- the purpose.

The signer then does the following, in order:

1. It verifies the signature, the purpose, the 60-second window, the admin
   list and its own replay set, independently of traceability-api. A traceability-api
   that forges or alters a request is refused here.
2. **Admit**:
   - it checks the chain first, and if the business is already a jeweler it answers that jeweler's id and does nothing else, so a retried click is safe;
   - otherwise member 1 submits an x/group proposal from the office policy carrying `MsgAdmitJeweler{name, business}`, with `exec: try`;
   - member 2 votes yes with `exec: try`, and the proposal executes;
   - it confirms that the jeweler exists;
   - member 3 sends the business 50 YML.

   This is the sequence `scripts/devnet/traceability-admit.sh` runs today with
   `OFFICE_VOTES=2` and `FUNDER=jewel-office-3`, ported to Go.
3. **Suspend and reinstate**: the same two-vote sequence with
   `MsgSuspendJeweler{jeweler, reason}` or `MsgReinstateJeweler{jeweler}`,
   followed by a check of the status.
4. It answers with `{tx, jeweler}`, or with the chain's refusal as plain text
   plus the raw log.

Every transaction is confirmed in a block, not at CheckTx. The memory note
`group-proposal-seeder-mechanics` lists the failure modes; success is that
the proposal executed and has been pruned.

traceability-api then records the outcome:
- on success, `submitted → admitted`, storing the jeweler id and the tx;
- on refusal or timeout, the application stays `submitted`, and the admin sees the reason with the raw error behind a disclosure, as `FailureNote` does today.

`jewel-admit` stays installed as the fallback for when the app or the signer is
down.

### History

Every state change and every admin action appends a record to the application:
- the state before and after;
- the actor's address and admin name;
- the time;
- the note or reason;
- the tx where there is one.

Records are never edited or deleted. Retention deletes documents, not history.

## 3. Documents, email, failures, limits

### Storage

Documents are stored as
`/var/lib/yamale-traceability/kyb/<application id>/<file id>.bin`, with a JSON
manifest of the kind, the detected type, the size, the original sha256 and the
upload time. The original filename is not kept.

Encryption:
- AES-256-GCM with a random nonce for each file.
- The 32-byte key is read from `/etc/yamale/traceability-kyb-key`, mode 640, owned by root with the yamale-traceability group, and generated by the installer if it is missing.
- The installer never prints the key.

Serving to admins:
- `Content-Type` is the detected type;
- `Content-Disposition: inline`;
- `Cache-Control: no-store`;
- `X-Content-Type-Options: nosniff`;
- `Content-Security-Policy: sandbox; default-src 'none'; img-src 'self' data:; object-src 'self'`.

The app opens a document from a `blob:` URL that it builds after a signed fetch,
so no document has a URL that works without a signature.

The chain never sees a document, a hash of one, or any personal field. It
learns only what it learns today: the jeweler's public name and business
account.

### Retention

A nightly job deletes:
- **admitted** applications' documents 5 years after the jeweler stops existing (today a jeweler never stops existing, so they are kept);
- **refused** applications' documents 12 months after the refusal;
- **drafts and `more_needed`** applications with no activity for 90 days: the application becomes `abandoned` and its documents are deleted.

The job logs counts, never names. History is kept.

### Backup

The existing `yamale-traceability-backup.timer` already covers
`/var/lib/yamale-traceability`, so documents are backed up as ciphertext. The key
file is deliberately left out of that backup. The operator keeps a copy offline
(the installer says so once, when it creates the key). The backup still sits on
the same machine as the data, as already noted in gaps.md.

### Email

- traceability-api sends through Mailcow on yamale-us: SMTP submission on port 587, STARTTLS, from the mailbox `no-reply@yamaletech.com`.
- The password is in `/etc/yamale/traceability-smtp`, mode 640, owned by root with the yamale-traceability group.
- The admin's notification address is a flag (`--admin-email`).
- Messages are queued in an outbox directory and retried with backoff for 48 hours. A failed send never undoes a state change.
- The admin's application view shows "email not delivered" when the last message to the applicant failed for good.
- Each message is plain text in the applicant's language, which is the one the app was in when they submitted.

### Limits

**The gate**:
- the zone `yamale_traceability_write` (30 requests a minute) is extended to the new `PUT` and `DELETE` routes and the upload `POST`;
- a new zone, `yamale_traceability_upload` (6 requests a minute per IP), covers `/v1/applications/mine/files`;
- `client_max_body_size 10m` applies on that path only;
- `/v1/admin/` uses the write zone for `POST` and a read zone (60 a minute) for `GET`.

**traceability-api**:
- one open application per account;
- at most 5 files and 25 MB per application;
- at most 50 new drafts per 24 hours across the whole service. Above that it answers 503, and the admin is emailed once.

### Failure handling

- **Upload refused for its type or size:** the slot says why, in plain words.
- **Submit attempted while incomplete:** the button is disabled and the missing items are listed.
- **The signer is unreachable:** the approve button answers "The office could not be reached; nothing was changed on the chain", and the application stays `submitted`.
- **The chain refuses:** the admin sees the chain's reason in plain words, with the raw log behind a disclosure.
- **The SMTP server is down:** the change stands, the message waits in the outbox, and the delivery state is shown.

### Testing

**Go (traceability-api):**
- every legal and illegal state transition;
- applicant requests that are unsigned, stale, from the wrong account, or for another account's application;
- admin requests from non-admins (must get 404), outside the 60-second window, replayed, or with the action body altered;
- detection of PDF, JPEG and PNG, and refusal of a `.pdf` that is really HTML;
- size caps;
- the encryption round trip, and that a wrong key fails;
- retention on a fake clock;
- the outbox retrying and giving up;
- migration of a spooled application.

**Go (jewelofficed):**
- the same signature checks, independent of traceability-api;
- refusal of a body changed after signing;
- a retried admit that finds the business already a jeweler, and does nothing;
- the two-vote sequence against a stub chain;
- the chain refusing, reported as plain text plus the raw log.

**TypeScript (app):**
- the form rules and the upload rules, including EXIF removal from a JPEG fixture;
- the status display for each state;
- the Admin tab shown only when `whoami` succeeds;
- canonical action JSON byte-identical to Go's, pinned in `testdata/vectors.json` like today's signatures.

**End to end, on a local devnet:**
- apply with two documents;
- the admin asks for more;
- the applicant resubmits;
- the admin approves, and the jeweler exists on the chain with 50 YML;
- the admin suspends it, and its stock sale is refused, then reinstates it;
- every email lands in a local SMTP sink.

Then deployed to yamale-us and walked through on traceability.yamaletech.com with
the user.

## Deploy

`install-traceability.sh` gains:
- the KYB key;
- the SMTP password prompt, or a check that the file already exists;
- the admin list;
- the `jewelofficed` binary and unit;
- the move of the `/opt/yamale/jewel-office` keyring to the `yamale-jewel-office` user.

The `yamale-traceability-backup` timer keeps covering that keyring.

`deploy-gate.sh` gains the new zones and paths. The monitor gains the unit
`yamale-jewel-office`. The signer has no public probe.

## Out of scope, recorded in gaps.md

- automatic registry or sanctions screening, or a third-party KYB provider;
- separate reviewers with separate office votes, which would make the 2-of-3 a real one;
- office keys moved onto the members' own devices;
- editing the admin list from the app;
- an off-machine backup for the KYB documents and their key.
