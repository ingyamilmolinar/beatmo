# Groups, sharing and a minimal backend

Status: DESIGN, revision 6 (2026-10-24), final draft for approval.
Companion: the API schema, [`2026-10-24-backend-api.graphqls`](2026-10-24-backend-api.graphqls)
(it moves to `src/share/api/schema/` when built).

- **Revision 6** (owner review): **only admins invite** is now the owner's decision, not an author's
  call. Adds "Operations at a glance" (every query and mutation with its input, output, permission
  and refusals) and "Code shape" (the main packages, types and functions on each side).
- **Revision 5** (owner review): the server and the game are **separate Go modules** (`src/backend`,
  `src/go`) that share only a third, standard-library-only module (`src/share`). The compiler, not a
  test, keeps server code out of the game and game code out of the server. See "Code boundaries".
- **Revision 4** (owner review):
  - Groups get three roles, admin, writer (read/write) and reader (read only), checked through
    **permissions** that are listed in one table, so the roles can be refined later without touching
    handlers.
  - The group owner is gone. A group can have several admins and always has at least one, and its limits
    come from the best tier among its admins.
  - The API is **GraphQL**: gqlgen generates the server and genqlient the Go client. Only browser
    redirects, raw blob bytes, the Stripe webhook and the health check stay plain HTTP.
  - Log appends become one mutation per entry kind, each with its own permission.
- **Revision 3** (owner review): no IPFS node and no VM. Blobs live in Cloud Storage under
  IPFS-compatible addresses, and the API runs on Cloud Run behind the existing load balancer, routed
  by path on `beatmo.io`. The group log is a server-numbered table and there is one key per group (no
  rotation). Accounts are the bare minimum (register, verify email, log in, log out, plus Google sign-in),
  groups have an owner and no roles, and the API process is stateless. Postgres access is type-safe and
  generated from the migrated schema; migrations are deterministic, idempotent and run at startup.
  Docker Compose mirrors every cloud component locally.
- Revision 2: browser HTTP via `fetch`, WASM budget, rotation, desktop join by code, tiers, retention.
- Revision 1: the IPFS-first exploration of 2026-10-23/24.

## Intent

Beatmo users want to share circuits and instruments with friends: someone creates a **group**, sends
a **link**, friends join, and anything shared into the group reaches every member, including members
who were offline when it was shared. Anyone can **publish** a circuit or instrument to a public
catalog. Later phases add direct peer-to-peer transfer (desktop) and live co-editing (CRDTs, premium).

The *why* for the shape chosen:

- **Offline first.** Every computation already happens on the client. Sharing must not change that:
  the app works fully without the network, and everything it fetched stays usable offline.
- **Peer-to-peer ready, not peer-to-peer yet.** The browser cannot run an IPFS or libp2p node inside
  the WASM budget, so phase 1 is client-server. What makes later P2P a transport change rather than a
  redesign is in the data: objects are addressed by their hash (IPFS CIDv1, which Beatmo's
  content-addressed documents already are), log entries are signed by the device that wrote them, and
  merging two logs is a set union. Any peer, mirror or IPFS gateway can serve the same bytes later; the
  hash proves they are right.
- **The backend holds references, metadata and ciphertext.** It never computes anything a client
  could, and it cannot read group content.
- **Small, stateless and cheap.** One Cloud Run service, one Cloud SQL database, two buckets, the
  load balancer that already serves the site. No VM, no Kubernetes, no Terraform.

## Goals

1. Register with email + password, verify the email, log in, log out. Or sign in with Google.
2. Create a group, invite by single-use link, join, leave. Each member has a **role**:
   - **admin**: manage invites, roles and members, the group itself, and anyone's shares;
   - **writer**: read and share;
   - **reader**: read only.

   The creator is the first admin. An invite fixes the role it grants (writer or reader).
3. A **member cap** per group, from the best tier among its admins, checked when an invite is redeemed.
4. Share a circuit or an instrument into a group; every member receives it on their next sync, with
   no requirement that the sharer is online then.
5. Publish a circuit or instrument publicly; browse the catalog, including offline from the last copy
   the client fetched.
6. Premium tier through Stripe Checkout.
7. Fully scripted Google Cloud infrastructure, DNS included, and a local Docker Compose stack that
   runs the same API image against a stand-in for every cloud component.
8. A **typed API**: one GraphQL schema, which generates the server's resolver interfaces and the Go
   client's request functions and types. A change that breaks an operation a released client sends
   fails a test.

## Non-goals (this revision)

- Direct peer-to-peer transport, an IPFS node, key rotation (phase 2).
- Live co-editing / CRDT merge of one circuit (phase 3; the log format must not block it).
- Custom or per-group roles, per-object permissions, roles signed into the log (phase 2; see
  "Refining roles later").
- GraphQL subscriptions (sync is a pull), GraphQL file uploads (blob bytes are plain HTTP), a JS/TS
  client (the schema allows generating one later).
- Email change, multi-factor auth, account recovery other than password reset.
- Sharing **WAV-based** content. Phase 1 objects are ≤ 1 MiB (every circuit and synth instrument
  document); a circuit whose rows depend on a WAV is refused with a clear message.
- Server-side rendering, search ranking, recommendations, moderation tooling beyond delisting.

## Anti-goals (would be a failure even if it works)

- The app needing the backend to start, play, edit, import or export anything local.
- Group content readable by the backend. It stores group objects and log entries as ciphertext only.
- Any state in the API process that a request depends on (see "Stateless").
- A leaked invite link granting more than one membership.
- Hand-written SQL strings in application code, or a schema that can drift from the code using it.
- A migration whose result depends on when, how often or by which instance it ran.
- An authorisation check that names a role outside the role → permission table, or a group with
  members and no admin.
- A schema change that breaks an operation a released client sends.
- Server code being *importable* from the game, or game code from the server, at all. A test that
  catches it after the fact is not enough.
- Any GraphQL client runtime that links `net/http` into the WASM build. Importing it adds 544 KiB
  gzip even to a minimal js/wasm binary (measured 2026-10-24: 906,288 → 1,463,027 bytes), and the
  budget has about 120 KiB of headroom.
- Dependency sprawl: an ORM, a web framework, a second infra language, a vendor SDK where a REST
  call or a standard protocol does the job.
- A received share replacing the user's open circuit without them asking (import is
  replace-not-merge).

## Constraints from the existing codebase

- **WASM budget.** `TestWasmAssetSizeBudget` (`internal/deploy/asset_size_test.go`) allows 6.40 MiB
  gzip with ~0.12 MiB headroom. The js/wasm build links no `net/http` and no `crypto/tls` (the test's
  comments record removing them). So:
  - **HTTP in the browser goes through `fetch` via `syscall/js`**, behind a js/notjs build-tag pair
    (desktop uses `net/http`).
  - **Crypto is the Go standard library on both platforms.** `crypto/hpke` (Go 1.26), `crypto/ed25519`
    and `crypto/ecdh` are new to the WASM; the first implementation step measures their gzip cost and
    raises the budget by exactly that amount, with a comment citing it.
  - **No backend code or dependency can reach a client binary.** The server is its own Go module
    that the game's `go.mod` never requires, so such an import does not compile (see "Code
    boundaries").
- **Browser offline-first needs a service worker, which does not exist yet.** There is none in
  `src/js` or in the deploy manifest, so the page does not load offline today. Phase 1 adds one
  (`sw.js`: precache the deploy manifest, network-first for `index.html`), listed in
  `deploy-wasm-gcs.sh`'s arrays (`manifest_drift_test.go` enforces it). The client also calls
  `navigator.storage.persist()`. Without that, browser storage can be evicted: Safari clears
  script-written storage after 7 days without a visit, except for home-screen apps. So a browser
  device must be re-registrable at sign-in.
- **Background work** goes through `async.Registry`. A new standing pool is the 3-edit change (table
  row, `standingFootprint`, `minDefaultBudget`), and since it is reached from user actions it never
  uses `MustGet`.
- **Stores** follow the `userprefs.NewInstrumentStore` js/notjs split; the object cache needs
  **IndexedDB** in the browser.
- **Events**: every new `hooks.Kind` needs `KindAll`, an `ActionRegistry` entry with a lowercase tag,
  an `eventlogger` formatter, and the `NumNonVerbose` check.
- **UI**: menus through `MenuCard`/`menuCardEntry`, text through `TextField`, every new surface gets a
  scene in `scene_catalog.go` and is visually verified.
- **Import safety.** Received bytes are hash-verified, decrypted, then go through the existing strict
  import path (`Game.Import` / `ApplyInstrumentBundle`). A peer-provided `url` WAV source is never
  fetched.
- **Schema versioning.** Every new client JSON document (log entry, catalog listing, envelope)
  carries `schema.Envelope` and follows the strict reader rule.

## Architecture

```
  client (desktop / browser)  — device keys · group store · object cache · sync pool · inbox UI
        │  HTTPS, same origin as the web app, every call optional (failure = "offline")
        ▼
  https://beatmo.io  — existing global HTTPS load balancer, one URL map
        ├── /v1/*    → serverless NEG → Cloud Run "beatmo-api" (stateless, scales to zero)
        │                 POST /v1/graphql, plus the few plain-HTTP routes ("API")
        │                                 ├── Cloud SQL Postgres (built-in Cloud SQL connection)
        │                                 ├── GCS beatmo-group-objects (private, via the API only)
        │                                 ├── SMTP provider (verification / reset mail)
        │                                 └── accounts.google.com (OIDC)
        ├── /ipfs/*  → backend bucket beatmo-public-objects (Cloud CDN)
        └── default  → backend bucket www-beatmo-io-static (the site, unchanged)
```

Routing by path on the app's own origin means no new certificate, no `api.` DNS record, and no
CORS preflight before every authenticated call.

### Stateless

The API process keeps nothing between requests that affects a result. Any instance can serve any
request, instances can be killed at any moment, and there is no session affinity and no local disk.

- **All durable state is in Postgres or a bucket**: accounts, sessions (refresh tokens),
  rate-limit windows.
- **What travels with the client is signed or encrypted by the server**: access tokens, email
  verification and password-reset tokens, the Google sign-in state.
- **The only allowed in-memory data is a cache that can be fetched again**: Google's signing keys,
  held by `go-oidc`.
- **Enforced locally**: Compose runs the API container with `read_only: true` and **two replicas**
  behind the proxy, so a hidden write or instance affinity fails on a developer machine first.

### Identity and accounts

**Device keys.** Each device generates an Ed25519 key pair (signing log entries) and an X25519 key
pair (receiving group keys) on first sign-in and keeps them locally (desktop: a 0600 file in the
config dir; browser: IndexedDB). A session is bound to one device.

**Email + password.** Each step below is a GraphQL mutation (see the schema).
- *Register*: `register {email, password, displayName}`. It always succeeds, so it never reveals
  whether an email exists.
  - New email: create the user unverified and mail a verification link.
  - Unverified existing email: replace the password hash (the latest registration wins) and mail a
    new link.
  - Verified existing email: mail "someone tried to register with your address".
- *Verify*: the link is `https://beatmo.io/verify#t=<token>`. The app asks for the password and calls
  `verifyEmail {token, password, device}`. Asking for the password defeats account pre-hijacking: an attacker who
  pre-registered your address with their password gains nothing when you click the link, because
  you don't know their password and use reset instead. Verifying also logs in.
- *Log in*: `login {email, password, device}` returns `EMAIL_NOT_VERIFIED` until the email is
  verified, and the client then offers `resendVerification`.
- *Log out*: `logout` deletes the session row.
- *Reset*: `forgotPassword {email}` always succeeds; `resetPassword {token, password}` sets the
  password and deletes **all** of that user's sessions.
- *Devices*: `device` is `{name, signPub, boxPub}`. The server upserts it by `(user, signPub)`, so a
  device that signs in again keeps its id and its envelopes.
- Passwords: 8–128 bytes and no composition rules (NIST 800-63B). Hashed with argon2id from
  `golang.org/x/crypto/argon2` (OWASP parameters m=19 MiB, t=2, p=1), stored in PHC string format.

**Google.** OIDC authorization code flow with PKCE (`golang.org/x/oauth2` +
`github.com/coreos/go-oidc/v3`), scopes `openid email profile`. Both platforms use the same two
server routes:
1. The app generates a secret `v` and calls `GET /v1/auth/google/start?challenge=sha256(v)&return=web`
   (or `return=loopback:<port>` on desktop).
2. The server puts nonce, PKCE verifier, challenge and return target into an **encrypted, HttpOnly,
   SameSite=Lax cookie** (10 min; the stateless replacement for a server-side session) and redirects
   to Google.
3. `GET /v1/auth/google/callback` checks state and nonce, exchanges the code, verifies the ID token,
   and requires `email_verified`. It redirects to `https://beatmo.io/#handoff=<token>` (browser) or
   `http://127.0.0.1:<port>/?handoff=<token>` (desktop, RFC 8252 loopback; no URL scheme, no
   installer change). The handoff token is 60 s, signed and bound to the challenge.
4. The app calls the `exchangeHandoff {handoff, verifier: v, device}` mutation and gets its tokens. A leaked
   handoff URL is useless without `v`, which never leaves the app.

Account linking, by Google `sub` first and then by email:
- `sub` already linked: log in.
- A verified email account exists: link `sub` to it.
- An *unverified* email account exists: take it over. Link `sub`, mark it verified and clear the
  password hash, so a pre-hijacker's password stops working.
- Otherwise: create the account.

**Tokens.**
- **Access token**: a JWT (HS256, 15 min, claims `sub`, `sid`, `did`), verified without a database
  read.
- **Refresh token**: 256 random bits, stored as its SHA-256 in `sessions`. It rotates on every
  refresh and expires 30 days after last use.
  - The session keeps the previous hash too. Presenting a rotated-out token deletes the session,
    since it signals a stolen token (OAuth 2.0 security BCP).
  - So refreshes must never race. The client single-flights them, in the browser across tabs with
    `navigator.locks.request("beatmo-refresh", …)`, because tabs share one session through IndexedDB.
- **Logout** deletes the session row: the refresh token dies at once and the access token within
  15 min (accepted trade-off of stateless access tokens).
- **Email, reset, handoff and state tokens** use the same keys with a `purpose` claim. A reset token
  carries a fingerprint of the current password hash, so it is single-use with no table.
- **Keys**: one `TOKEN_KEYS` secret, a JSON list of `{kid, key}` where the first entry signs and all
  entries verify, so keys rotate by prepending. Per-purpose subkeys are derived with `crypto/hkdf`.
- **Library**: `github.com/go-jose/go-jose/v4` handles both JWS and the state-cookie JWE. It is
  already a dependency of `go-oidc`, so it adds no new dependency.

**Rate limits** are kept in Postgres, so they don't need process state. A fixed-window counter
(`INSERT … ON CONFLICT DO UPDATE SET hits = hits + 1 RETURNING hits`) is keyed by client IP and by
email. It guards the `register`, `login`, `verifyEmail`, `resendVerification`, `forgotPassword`,
`exchangeHandoff` and `redeemInvite` mutations, Google start, and the blob PUTs. Over the limit, a
mutation gets a top-level `RATE_LIMITED` error with `retryAfterSeconds`, and HTTP routes get a 429
with a `Retry-After` header. The client IP is the
address the Google load balancer appended to `X-Forwarded-For`, never the leftmost entry, which the
client controls. Expired windows are deleted at startup.

### Objects and addresses

- An object is the exact bytes Beatmo already exports (a project file or an instrument bundle) or,
  for a group, their ciphertext. ≤ 1 MiB, enforced with `http.MaxBytesReader`.
- **Address: CIDv1, raw codec (0x55), sha2-256 multihash, base32**, encoded by hand (~40 lines, no
  go-cid). For a public object the digest *is* the SHA-256 of the exported bytes, so it converts to
  and from Beatmo's existing hashes. A group object's CID is the hash of its ciphertext, which is new
  for every share (random nonce).
- The server always recomputes the CID of an upload and refuses a mismatch. Writes use
  `ifGenerationMatch=0`, so an object is never overwritten (content addressing makes a repeat upload
  a no-op).
- **Public objects** go to `beatmo-public-objects` at `ipfs/<cid>` and are served by the load
  balancer at `https://beatmo.io/ipfs/<cid>`: the URL shape of an IPFS trustless gateway.
  Uploads are stored as `application/vnd.ipld.raw` with `Content-Disposition: attachment`. The backend
  bucket adds `X-Content-Type-Options: nosniff`, `Content-Security-Policy: sandbox` and
  `Access-Control-Allow-Origin: *`. User content on the app's own origin must never render, or an
  uploaded HTML file becomes stored XSS next to the device keys. `Cache-Control: public,
  max-age=86400`, so a delisted object can stay in the CDN for a day; an emergency uses a manual
  cache invalidation.
- **Group objects** go to the private `beatmo-group-objects` at `g/<group>/<cid>`, readable only
  through `GET /v1/groups/{id}/objects/{cid}` with `GROUP_READ`.
- **Upload, then reference.**
  - A blob `PUT` stores bytes only (`CONTENT_WRITE` for a group; signed in for public). The
    `shareObject` / `publish` mutation then checks that the blob exists and creates the row in the
    same transaction, together with the log entry for a share.
  - So a row always means "shared" or "published". A blob that was uploaded but never referenced is
    an orphan for `gc-orphans.sh`.
  - Quota is checked at the PUT (existing rows + this size) and again in the mutation.
- **Fetch order** (client): local cache → `beatmo.io` → (public objects only) configured public IPFS
  gateways. Every response is re-hashed and a mismatch is discarded. Adding an IPFS node later only
  makes that last step useful; it changes no client code.
- Blob access from the API is the GCS JSON API over plain HTTPS: three calls (insert, get, delete; get
  without `alt=media` returns only the metadata, which is how `shareObject` checks the blob exists),
  authenticated with `golang.org/x/oauth2/google`'s default token source, which is already a
  dependency for Google sign-in. No Cloud Storage SDK.

### Groups

- **One symmetric key per group** (AES-256-GCM, random 96-bit nonce; the group id and epoch as
  associated data). The creator generates it, and each member device gets its own copy wrapped with
  **`crypto/hpke`** (X25519 + HKDF-SHA256 + AES-256-GCM, RFC 9180). The backend stores these
  wrapped copies ("envelopes") but cannot open them.
- **No rotation in phase 1.** Every channel content travels on is gated by the server:
  - group blobs and the log are served only to current members;
  - removing a member or deleting a device revokes its access immediately, and it never learns new
    CIDs.

  Rotation only protects against ciphertext reaching non-members, which starts with phase 2's direct
  transport. The format carries `epoch` (always 0) so rotation can be added without migrating
  anything. Consequence: someone who joins late sees the whole history.
- **New devices wait for a key.** A device with no envelope gets one the next time any member device
  holding the key syncs. `Group.devicesAwaitingKey` lists member devices still lacking an envelope,
  and `addEnvelopes` supplies them. This latency is accepted.
- **Device-added notice.** `addEnvelopes` takes the wrapping client's signed `device_added` entry and
  stores it in the same transaction as the envelopes. Every member then sees "Ana added a device". This makes a fake
  device injected by a compromised backend visible. Preventing it outright (approving new devices
  from an existing one) is phase 2 hardening.
- **The log.** Each entry is a signed, then encrypted, JSON document `{schema_version, group, epoch,
  author_device, author_sign_pub, kind: "share" | "unshare" | "device_added", object: cid, devices
  (device_added only: the device ids given a key), meta: {kind, title, size}, created_at}`, signed
  with the author's Ed25519 key.
  - The backend stores entries in `group_log` under a **per-group sequence number** and clients sync
    with `group(id) { log(after: <seq>) }`: one request, no DAG walk.
  - **There is no generic "append".** Each kind has its own mutation and permission: `shareObject`
    (`CONTENT_WRITE`), `unshareObject` (`CONTENT_WRITE` on your own share, `CONTENT_MODERATE` on
    anyone's), and `addEnvelopes` (`KEY_DISTRIBUTE`).
  - To check those permissions without reading content, the server stores **plaintext copies** of the
    entry's `kind` and `object` CID next to the ciphertext. It already learns both from the blob
    upload and delete. Clients drop any entry whose decrypted, signed fields differ from the
    plaintext copies.
  - Appending takes the group row's lock (`UPDATE groups SET log_seq = log_seq + 1 … RETURNING`) in the
    same transaction as the insert, so the numbers are contiguous and commit in order. With a plain
    global identity column, a reader could see 11 before 10 had committed and skip 10 forever.
  - An entry's identity is its hash, and merging is a set union. Phase 2 peers can exchange entries
    directly, and phase 3 adds entry kinds without changing the format.
  - **Unknown kinds are kept, not refused.** A client that meets a kind it does not know (from a
    newer client) keeps the entry in its copy of the log and advances its cursor past it, but acts on
    nothing in it. A strict decode there would stall an old client's sync forever on the first new
    kind. The same holds for values of the `LogKind`, `Permission` and `GroupRole` enums the server
    sends: genqlient generates them as string types, so an unknown value decodes, and the client
    ignores a permission it does not know.
- **Roles and permissions** (RBAC). Each membership has one role. Code checks a **permission**, never
  a role. The only place roles are named in authorisation logic is the table below, in
  `src/backend/internal/authz` (`var rolePermissions = map[model.GroupRole]PermSet{…}`):

  | Permission | Allows | reader | writer | admin |
  |---|---|:-:|:-:|:-:|
  | `GROUP_READ` | see the group, members, log; fetch objects; own envelope | ✓ | ✓ | ✓ |
  | `KEY_DISTRIBUTE` | wrap the key for member devices lacking it (`addEnvelopes`) | ✓ | ✓ | ✓ |
  | `CONTENT_WRITE` | upload, `shareObject`, `unshareObject` on own shares | | ✓ | ✓ |
  | `CONTENT_MODERATE` | `unshareObject` on anyone's share | | | ✓ |
  | `MEMBERS_INVITE` | `createInvite`, `revokeInvite`, `Group.invites` | | | ✓ |
  | `MEMBERS_MANAGE` | `setMemberRole`, `removeMember` | | | ✓ |
  | `GROUP_MANAGE` | `renameGroup`, `deleteGroup` | | | ✓ |

  `leaveGroup` needs only membership. Readers hold the group key, because they must decrypt.
  "Read only" is therefore enforced by the server, which sequences every write. Phase 1 has no direct
  transport, so the server's word is final.
- **The check.** `authz.Require(ctx, q, groupID, perm)` reads the caller's `group_members` row: one
  primary-key lookup.
  - Roles are never put in the access token, so a demotion or removal applies to the caller's
    **next request**, not 15 minutes later.
  - A non-member gets `NOT_FOUND` (no existence oracle). A member missing the permission gets
    `FORBIDDEN`.
  - Clients read `Group.myPermissions` to decide what to offer, so they never encode the table
    themselves.
- **Admins.** A group can have several. All admins are equal, and the creator is not special: an
  admin can demote or remove another admin.
- **Invariant: a group with members always has at least one admin.**
  - `setMemberRole` and `removeMember` refuse with `LAST_ADMIN` when the change would break it.
  - `leaveGroup` refuses with `LAST_ADMIN` for the only admin while others remain; the UI asks them
    to promote someone first.
  - The last member leaving deletes the group.
  - Account deletion cannot be refused, so it promotes for them: the earliest-joined writer, or if
    there is none the earliest-joined reader.
- **Concurrency.** Every transaction that changes membership, roles or the log first locks the group
  row (`SELECT … FROM groups WHERE id = $1 FOR UPDATE`). The log's `log_seq` increment takes that
  same lock. `authz.Require` runs inside the transaction, after the lock. So two admins demoting each
  other at once are serialised: the second sees one admin left and gets `LAST_ADMIN`. Guard:
  `TestLastAdminInvariantUnderConcurrency`.
- **Invites.**
  - Only `MEMBERS_INVITE` (admins) can create, list or revoke an invite (owner decision, revision 6).
    Each invite fixes the role it grants: writer (the default) or reader, never admin.
  - The code is a random 128-bit value in base32, carried in the link fragment
    (`https://beatmo.io/join#<code>`), so it never reaches CDN or request logs. Desktop's Join
    `TextField` accepts the pasted link or the bare code.
  - The backend stores only the code's SHA-256, plus a uuid `id` used for revoking. The code is
    returned once, by `createInvite`.
  - An invite is single-use and expires after 7 days. Redemption locks the group row, then checks
    unused / unrevoked / unexpired status and the member cap.
- **Refining roles later** (no handler changes):
  - *A new built-in role*: add the enum value in a migration (`ALTER TYPE group_role ADD VALUE IF
    NOT EXISTS`), in the GraphQL enum and in the table above. `TestGraphQLEnumsMatchPostgres` and
    `TestPermissionTableIsComplete` fail until all three agree.
  - *A new permission*: add it to the GraphQL `Permission` enum and the Go constants, and give it a
    column in the table above.
  - *Custom per-group roles*: add a `group_roles (group_id, name, permissions text[])` table and
    point `group_members.role` at it. `authz.Require`'s signature does not change.
  - *Roles verifiable by peers* (phase 2, direct transport): role changes become signed log entries,
    so a peer can check an author's permission at that `seq` without the server.
- **Receiving.** New shares land in an **inbox** (a `MenuCard` list) with three actions: open (the
  normal import path, as a new circuit), add instrument to library, or dismiss.

### Tiers, quotas and retention

- **Member cap and group storage quota** follow the **best tier among the group's admins**
  (placeholders: free 8 members / 50 MiB, premium 50 / 1 GiB; one Go table, also served as
  `Query.tiers`). A premium admin lends the group their limits. If they stop being admin, the limits
  drop, and nothing is deleted. Over the cap, existing members stay and new redemptions
  are refused; over quota, uploads are refused. Nothing is ever deleted for being over.
- **Public storage quota** per publishing account, from the same table.
- **Premium**: a monthly Stripe subscription.
  - The mutations `startCheckout` and `openBillingPortal` (cancel, card change) return Stripe URLs.
    The plain-HTTP `POST /v1/billing/webhook` updates `entitlements` (`period_end`, cancellation).
  - Uses `github.com/stripe/stripe-go` (official, no heavy dependencies; its `webhook.ConstructEvent`
    verifies signatures).
  - Billing is disabled when the Stripe keys are unset, which is the local default.
- **Retention**:
  - a group blob lives until `unshareObject` (its sharer, or an admin) or until the group is
    deleted;
  - a public blob is deleted when its **last** listing is delisted (two publishers can list the same
    bytes; see "Public catalog");
  - a blob whose delete failed after the database commit is an orphan. It is harmless and costs
    pennies, and `deploy/backend/gc-orphans.sh` lists and removes orphans on demand.

### Account deletion and export

- **`deleteAccount`** removes the user. It needs the password, or for a Google-only account a session
  created in the last 10 minutes, so a stolen access token alone cannot delete an account.
  - Where they were the only admin, the earliest-joined writer (else reader) is promoted. Groups
    where they were alone are deleted.
  - their devices, sessions and envelopes cascade;
  - their listings are delisted, and each blob no other listing references is deleted;
  - shares they made into groups stay with those groups, with the uploader set to null.
- **Export** is the `me` query with every field: account, devices, memberships, publications, and
  the CIDs they shared. The content itself is already on their devices.

### Public catalog

- **Publish**: upload with `PUT /v1/objects/{cid}`, then `publish {cid, kind, title, tags,
  licence}`, where the licence is `CC0-1.0` or `CC-BY-4.0`, chosen at publish. The publish screen
  states that published content cannot be recalled once others copy it. **Delist**: `delist {cid}`.
- **The same bytes can have several publishers.** Exports are deterministic, so two people publishing
  an untouched template produce the same CID, and that will be common. A listing is therefore keyed
  by `(cid, publisher_id)`:
  - each publisher has their own row (title, tags, licence), can delist only it, and is charged its
    size against their own quota;
  - publishing the same CID again *as the same publisher* is a no-op that returns the existing row;
  - the blob is shared and lives while any listing references it. `delist` and account deletion
    delete it once the transaction that removes the last row has committed;
  - the catalog returns every listing. The client groups entries by CID when it shows them.
- **Operator takedown**: `deploy/backend/takedown.sh <cid>` deletes every listing of that CID and its
  blob, then issues the CDN cache invalidation for `/ipfs/<cid>`. That is the whole moderation
  toolset in phase 1 (see Non-goals).
- **Browse**: `catalog(ifNoneMatch:)` returns the whole listing with an `etag`, or
  `notModified: true` when the client's copy is current. The client keeps the last copy and browses
  and searches it locally, offline. The etag is a hash of the listing's newest `published_at` and
  its row count, computed in one query. Pagination and a content-addressed snapshot are deferred
  until the listing approaches 1 MiB.

### API

**One GraphQL endpoint, plus a few plain-HTTP routes for what GraphQL does badly.** The schema is
[`2026-10-24-backend-api.graphqls`](2026-10-24-backend-api.graphqls): 4 queries, 27 mutations. It is
validated with gqlparser, and its type graph is checked to be acyclic.

| Route | Who | Why it is not GraphQL |
|---|---|---|
| `POST /v1/graphql` | per field (see schema) | everything else |
| `GET /v1/auth/google/start` · `/callback` | anyone | browser navigations and redirects, with a cookie |
| `PUT` · `GET /v1/groups/{id}/objects/{cid}` | `CONTENT_WRITE` · `GROUP_READ` | raw bytes up to 1 MiB; GraphQL would base64 them (+33 %) and needs the multipart-upload extension |
| `PUT /v1/objects/{cid}` | signed in | raw bytes (public blobs are read from `/ipfs/<cid>` on the bucket, not the API) |
| `POST /v1/billing/webhook` | Stripe | Stripe's own signed format |
| `GET /v1/healthz` | anyone | Cloud Run / Compose probe |

Routing is Go 1.22+ `net/http.ServeMux` patterns; there is no router library. Plain-HTTP errors are
`{"error": {"code", "message"}}`, using the same codes as GraphQL, with these statuses:
`UNAUTHENTICATED` 401, `FORBIDDEN` 403, `NOT_FOUND` 404, `BAD_INPUT` 400 (a CID that does not
match the bytes included), `QUOTA_EXCEEDED` 409, over 1 MiB 413, `RATE_LIMITED` 429.

- **Blob `PUT`**: raw bytes, `Content-Type: application/octet-stream`. Returns 201 when stored, or 200
  when that CID already exists (content addressing makes a repeat a no-op).
- **Group blob `GET`**: 200 with the bytes and `Cache-Control: private, max-age=31536000, immutable`.
  A CID's bytes never change.

#### Operations at a glance

The schema file is normative; this table is the summary. "Who" is the permission checked in the
caller's group. "Refusals" are the `error.code` values the payload can carry. Top-level errors
(`UNAUTHENTICATED`, `FORBIDDEN`, `NOT_FOUND`, `BAD_INPUT`, `RATE_LIMITED`, `INTERNAL`) can happen on
any of them.

**Queries**

| Query | Arguments | Returns | Who |
|---|---|---|---|
| `me` | none | `Me` (null when signed out): account, `tier`, `currentDevice`, `devices`, `memberships { role group }`, `publications`, `shares` | signed in |
| `group` | `id` | `Group` (null for non-members): `name`, `myRole`, `myPermissions`, `members { user role devices }`, `limits`, `myEnvelope`, `objects`, plus the three fields below | `GROUP_READ` |
| ↳ `Group.log` | `after: Int64 = 0`, `first: Int = 200` (1..500) | `LogPage { entries { seq kind object author authorDeviceId entry createdAt } cursor hasMore }` | `GROUP_READ` |
| ↳ `Group.devicesAwaitingKey` | none | `[MemberDevice { user device }]` | `KEY_DISTRIBUTE` |
| ↳ `Group.invites` | none | `[Invite { id role createdBy createdAt expiresAt }]`, null without the permission | `MEMBERS_INVITE` |
| `catalog` | `ifNoneMatch: String` | `Catalog { etag notModified entries }` | anyone |
| `tiers` | none | `[TierLimits { tier memberCap groupStorageBytes publicStorageBytes }]` | anyone |

**Mutations.** Every mutation that has arguments takes a single `input`. `DeviceInput` is
`{name, signPub, boxPub}` and `EnvelopeInput` is `{deviceId, epoch = 0, sealed}`.

| Mutation | Input | Returns | Who | Refusals |
|---|---|---|---|---|
| `register` | `email, password, displayName` | `AcceptedPayload { ok error }` | anyone | `INVALID_INPUT` |
| `resendVerification` | `email` | `AcceptedPayload` | anyone | none |
| `verifyEmail` | `token, password, device` | `SessionPayload { session { accessToken accessTokenExpiresAt refreshToken refreshTokenExpiresAt me } error }` | anyone | `TOKEN_INVALID`, `INVALID_CREDENTIALS` |
| `login` | `email, password, device` | `SessionPayload` | anyone | `INVALID_CREDENTIALS`, `EMAIL_NOT_VERIFIED` |
| `exchangeHandoff` | `handoff, verifier, device` | `SessionPayload` | anyone | `TOKEN_INVALID` |
| `refreshSession` | `refreshToken` | `SessionPayload` | anyone | `TOKEN_INVALID` |
| `forgotPassword` | `email` | `AcceptedPayload` | anyone | none |
| `resetPassword` | `token, password` | `AcceptedPayload` | anyone | `TOKEN_INVALID`, `INVALID_INPUT` |
| `logout` | none | `AcceptedPayload` | signed in | none |
| `deleteAccount` | `password` (optional) | `AcceptedPayload` | signed in | `INVALID_CREDENTIALS`, `REAUTH_REQUIRED` |
| `deleteDevice` | `deviceId` | `AcceptedPayload` | signed in, own device | none |
| `createGroup` | `name, envelopes[]` | `GroupPayload { group error }` | signed in (becomes admin) | `INVALID_INPUT` |
| `renameGroup` | `groupId, name` | `GroupPayload` | `GROUP_MANAGE` | `INVALID_INPUT` |
| `deleteGroup` | `groupId` | `AcceptedPayload` | `GROUP_MANAGE` | none |
| `leaveGroup` | `groupId` | `AcceptedPayload` | member | `LAST_ADMIN` |
| `createInvite` | `groupId, role: InviteRole = WRITER` | `InvitePayload { invite code link error }` (code shown once) | `MEMBERS_INVITE` | none |
| `revokeInvite` | `inviteId` | `AcceptedPayload` | `MEMBERS_INVITE` in the invite's group | none |
| `redeemInvite` | `code` | `GroupPayload` | signed in | `INVITE_INVALID`, `ALREADY_MEMBER`, `GROUP_FULL` |
| `setMemberRole` | `groupId, userId, role` | `MemberPayload { member error }` | `MEMBERS_MANAGE` | `LAST_ADMIN` |
| `removeMember` | `groupId, userId` | `AcceptedPayload` | `MEMBERS_MANAGE` | `LAST_ADMIN` |
| `addEnvelopes` | `groupId, envelopes[], notice` | `LogEntryPayload { entry error }` | `KEY_DISTRIBUTE` | `INVALID_INPUT` |
| `shareObject` | `groupId, cid, entry` | `LogEntryPayload` | `CONTENT_WRITE` | `OBJECT_MISSING`, `QUOTA_EXCEEDED`, `INVALID_INPUT` |
| `unshareObject` | `groupId, cid, entry` | `LogEntryPayload` | `CONTENT_WRITE` (own share) or `CONTENT_MODERATE` | `INVALID_INPUT` |
| `publish` | `cid, kind, title, tags = [], licence` | `PublicObjectPayload { object error }` | signed in | `OBJECT_MISSING`, `QUOTA_EXCEEDED`, `INVALID_INPUT` |
| `delist` | `cid` | `AcceptedPayload` | the publisher | none |
| `startCheckout` | none | `UrlPayload { url error }` | signed in | `BILLING_DISABLED` |
| `openBillingPortal` | none | `UrlPayload` | signed in | `BILLING_DISABLED` |

Rules the table implies:
- `addEnvelopes` accepts an envelope only for a device of a current member that has none yet.
  Replacing an existing envelope is refused (`INVALID_INPUT`, `field` names it), because a member
  could otherwise overwrite someone's key with garbage.
- `shareObject` and `unshareObject` refuse an `entry` over 16 KiB (`INVALID_INPUT`).
- A mutation whose refusal list says "none" can still fail with the top-level errors above.

#### Wire protocol (what the client and server agree on)

- **Request**: `POST /v1/graphql` with `Content-Type: application/json` and the body
  `{"query", "operationName", "variables"}`. Only POST is accepted: no GET queries, no websocket,
  no multipart. That rejects form-encoded cross-site posts; there are no cookies on this route
  either.
- **Auth**: `Authorization: Bearer <access token>`. A missing or expired token is not an HTTP error.
  The request runs anonymously, `me` is null, and fields that need a user return `UNAUTHENTICATED`.
- **Response**: always HTTP 200 with `{"data", "errors"}` (GraphQL over HTTP, `application/json`).
  The exceptions happen before GraphQL runs: 400 for an unparsable body, 413 over 64 KiB, 415 for
  the wrong content type.
- **Errors, two kinds**:
  - *Expected refusals* are data. A mutation's payload carries `error: UserError {code, message,
    field}`, where `code` is the `ErrorCode` enum, so the generated client gets a typed switch.
  - *Everything else* is a top-level `errors[]` entry with `extensions.code`: `UNAUTHENTICATED`,
    `FORBIDDEN`, `NOT_FOUND`, `BAD_INPUT`, `RATE_LIMITED` (+ `retryAfterSeconds`), or `INTERNAL`
    (the message is masked, and the server logs the cause with a request id it returns in
    `extensions.requestId`).
- **Client transport behaviour** (both platforms):
  - a 15 s timeout;
  - a network error, a 5xx or a timeout is "offline" (no error dialog);
  - on `UNAUTHENTICATED` with a refresh token, one single-flighted `refreshSession` and one retry;
  - `RATE_LIMITED` waits `retryAfterSeconds`.

#### Server (gqlgen)

- **`github.com/99designs/gqlgen`** (v0.17.98, 2026-10-22), schema-first. `go tool gqlgen` reads
  `src/share/api/schema/*.graphqls` and generates:
  - the executor;
  - the DTO structs (`api/gen/model`);
  - one resolver interface per type.

  The hand-written resolvers implement those interfaces, so a schema change that a resolver does
  not follow is a compile error.
- **Handler**: `handler.New(gen.NewExecutableSchema(cfg))` with `transport.POST{}` only, plus:
  - `extension.FixedComplexityLimit(300)`. Default complexity is 1 per field, and `log(first:)` is
    capped at 500. Depth needs no limiter, because the type graph has no cycles: `UserRef` and
    `Device` are leaves and `GroupObject` holds a `groupId`, not a `Group`.
  - `extension.Introspection{}` stays on. The schema is public in the repo anyway.
  - The parsed-query LRU cache (`lru.New[*ast.QueryDocument](1000)`). It is a re-derivable cache, so
    the "Stateless" rule allows it.
  - `SetErrorPresenter` maps typed store errors to codes and masks the rest; `SetRecoverFunc` turns a
    panic into `INTERNAL`.
- **Middleware** (`AroundOperations` / `AroundFields`):
  - access-token verification into the context (user, session, device);
  - rate limits keyed by operation and field name for the account mutations;
  - one request-log line per operation (name, duration, error codes; never variables, which carry
    passwords and tokens).
- **Resolvers** call store functions only. A mutation runs in one `db.Tx`.
  - A group-scoped resolver starts with `authz.Require` inside that transaction (after the group-row
    lock for writes).
  - A field that needs a permission the caller lacks (`Group.invites`) is null with a `FORBIDDEN`
    error on that path; the rest of the query still succeeds.
- **DTOs are gqlgen's models, never jet's.** Resolvers map `model.*` (jet) to `gen/model.*` (gqlgen).
  So the revision 3 rule "never serialise a database model" now holds by construction: a new column
  is invisible until the schema names it.
- **N+1** is bounded by the caps: members ≤ 50 per group, and a user's devices and groups are few.
  - `Group.members` loads members and their devices in one jet join.
  - Other nested lists resolve per parent.
  - A dataloader (`github.com/vikstrous/dataloadgen`, which the gqlgen docs recommend) is added only
    when a list becomes unbounded.
- **Local playground**: gqlgen's GraphiQL handler at `/v1/graphql/play`, only when
  `GRAPHQL_PLAYGROUND=1`. It is set in `local.env` and never in production.

#### Client (genqlient)

- **`github.com/Khan/genqlient`** (v0.8.1, May 2025; its release cadence is slow, but it is used
  only as a build-time generator).
  - The operations the app sends are files in `src/go/internal/backendclient/operations/*.graphql`.
  - `go tool genqlient` generates one typed Go function per operation, plus its response structs and
    enums.
  - A query that does not validate against the schema fails generation.
- **The runtime package is replaced**, because of the WASM budget.
  - Generated code imports `github.com/Khan/genqlient/graphql`, and that package imports `net/http`
    (see Anti-goals: +544 KiB gzip). genqlient has no option to change that import.
  - So `make gen-api` rewrites that one import line in the generated file to
    `src/go/internal/gqlclient`. That is Beatmo's copy of the few types the generated code uses
    (`Client`, `Request`, `Response`, the error list), about 80 lines, with genqlient's MIT notice
    kept.
  - Transports: `transport_notjs.go` uses `net/http` (desktop); `transport_js.go` uses `fetch` via
    `syscall/js` (browser).
  - The rewritten import is aliased (`graphql "…/internal/gqlclient"`). genqlient is a tool of
    `scripts/apigen`, not of the game, so a failed rewrite is a compile error, never a silent
    link (see "Code boundaries › Generators"). `TestWasmLinksNoNetHTTP` covers the transports.
- **Every operation a client ever shipped keeps working.**
  - genqlient's `export_operations` writes the exact operation documents.
  - `make gen-api` merges them into the committed, append-only `src/share/api/operations.lock.json`, keyed by
    hash (the `migrations.sum` idea applied to the API).
  - `TestReleasedClientOperationsStillValidate` validates every locked operation against the current
    schema with gqlparser, so removing or renaming a field an old desktop build still asks for fails
    CI.
  - An operation is retired from the lock only by hand, with a comment saying which client versions
    are no longer supported.
- **Schema evolution rule**: additive only, like migrations. Deprecate with `@deprecated(reason:)`;
  remove only after the lock no longer references the field. No unions or interfaces in phase 1, so a
  new type never meets an old client's exhaustive type switch.

#### Generation and checks

- `make gen-api`: gqlgen (server) + genqlient (client) + the import rewrite + the lock merge. The
  output is committed. CI regenerates it and fails on any diff (`TestGeneratedAPICodeIsCurrent`),
  like `make gen-db`.
- `TestRBACMatrix` makes one call per (mutation or permission-gated field) × (admin, writer, reader,
  non-member, signed out).
  - The expected outcome is derived from the permission table, never hand-written per test.
  - The test walks the schema's `Mutation` fields, so a new mutation without an entry fails.
- `TestPermissionTableIsComplete`: the GraphQL `Permission` enum, the Go constants and the table's
  columns are the same set, and every role has a row.
- `TestGraphQLEnumsMatchPostgres`: each GraphQL enum (`GroupRole`, `LogKind`, `ObjectKind`,
  `Licence`, `Tier`) equals its Postgres enum's labels, upper-cased, read from jet's generated
  constants. `InviteRole` must be a subset of `GroupRole`, without `ADMIN`.
- `TestSchemaHasNoCycles`: the check run on this draft, kept as a test.

### Data model (Postgres)

Constraint names are explicit, so error mapping and idempotent migrations can refer to them. Ids
are `uuid DEFAULT gen_random_uuid()`.

**Enum types.** Every closed set is a Postgres enum, so jet generates a Go type with constants for
it (`model.GroupRole_Admin`). A role in Go is then typed from the database up:

| Enum | Labels |
|---|---|
| `group_role` | `admin`, `writer`, `reader` |
| `log_kind` | `share`, `unshare`, `device_added` |
| `object_kind` | `circuit`, `instrument` |
| `licence` | `cc0_1_0`, `cc_by_4_0` |
| `tier` | `free`, `premium` |

- Created idempotently: `DO $$ BEGIN CREATE TYPE … AS ENUM (…); EXCEPTION WHEN duplicate_object
  THEN NULL; END $$`.
- Adding a label later: `ALTER TYPE … ADD VALUE IF NOT EXISTS`, in a file of its own, since a new
  label cannot be used in the transaction that adds it.
- Labels are never removed (expand-only).
- Code never relies on label order: tiers are compared through the Go limits table, not `max()`.
- If roles ever become per-group data, `ALTER COLUMN role TYPE text` is the migration (see
  "Refining roles later").

| Table | Columns (keys **bold**) |
|---|---|
| `users` | **id**, email `citext` UNIQUE, email_verified_at, password_hash (null for Google-only), google_sub UNIQUE null, display_name, created_at |
| `devices` | **id**, user_id → users CASCADE, name, sign_pub, box_pub, created_at; UNIQUE (user_id, sign_pub) |
| `sessions` | **id**, user_id → users CASCADE, device_id → devices CASCADE, refresh_hash UNIQUE, prev_refresh_hash null, created_at, last_used_at, expires_at |
| `groups` | **id**, name, created_by → users SET NULL, log_seq `bigint` default 0, created_at |
| `group_members` | **group_id** → groups CASCADE, **user_id** → users CASCADE, role `group_role`, joined_at, invited_by → users SET NULL, role_changed_at null, role_changed_by → users SET NULL; index (user_id); partial index (group_id) WHERE role = 'admin' |
| `invites` | **id**, code_hash UNIQUE, group_id → groups CASCADE, created_by → users SET NULL, role `group_role` CHECK (role <> 'admin'), created_at, expires_at, redeemed_by → users SET NULL, redeemed_at, revoked_at |
| `group_envelopes` | **group_id** → groups CASCADE, **device_id** → devices CASCADE, epoch, sealed `bytea` |
| `group_log` | **group_id** → groups CASCADE, **seq**, kind `log_kind`, object_cid null, author_user → users SET NULL, author_device → devices SET NULL, entry `bytea`, created_at; CHECK ((kind = 'device_added') = (object_cid IS NULL)) |
| `group_objects` | **group_id** → groups CASCADE, **cid**, uploader_id → users SET NULL, size, created_at; index (uploader_id) |
| `public_objects` | **cid**, **publisher_id** → users CASCADE, kind `object_kind`, title, tags `text[]`, licence `licence`, size, published_at |
| `entitlements` | **user_id** → users CASCADE, tier `tier`, stripe_customer_id UNIQUE, period_end, updated_at |
| `rate_limits` | **key**, **window_start**, hits |
| `goose_db_version` | owned by goose |

How the tables map onto the schema:
- **One table, one type.** `users` → `Me` / `UserRef`; `devices` → `Device`; `group_members` →
  `Member` / `Membership` (the join table *is* the membership, and its role is an attribute of the
  edge); `invites` → `Invite`; `group_log` → `LogEntry`; `group_objects` → `GroupObject`;
  `public_objects` → `PublicObject`; `group_envelopes` → `Envelope`.
- **Derived fields are computed, not stored**: `Group.myRole` / `myPermissions` (the caller's
  row + the table), `GroupLimits` (admins' tiers + `SUM(size)`), `devicesAwaitingKey` (members'
  devices anti-joined with envelopes).
- **Group and public objects are two tables** rather than one with a `scope` column, so every
  query's type says which one it reads. Delisting deletes the caller's `public_objects` row, and the
  blob with the last one.

### Code boundaries: three Go modules

The API server and the game have nothing to do with each other. They share only what is in
`src/share`, and the Go toolchain enforces that, so it does not depend on a test someone remembers
to run.

```
src/go/         module github.com/ingyamilmolinar/beatmo           the game, desktop + WASM (unchanged)
src/backend/    module github.com/ingyamilmolinar/beatmo/backend   the API server
src/share/      module github.com/ingyamilmolinar/beatmo/share     what both import; standard library only
scripts/apigen/   module (generator only)   genqlient, so the game's go.mod names no GraphQL module
scripts/modguard/ module (guards only)      belongs to neither side, like scripts/jsaudit
```

**Why modules and not build tags.**
- A build tag picks which files of a package compile. It does not stop an import, and a forgotten
  tag fails open: the file simply compiles in.
- With one module, the one `go.mod` would list pgx and gqlgen next to Ebiten, and every check would
  be after the fact.
- With modules, each side has its own `go.mod` and `go.sum`, so its dependency list *is* its
  boundary.
- Tags stay where they already belong: the js/notjs transport pair inside the game. The game
  already carries `test`, `noscenes`, `parityfatal` and js/notjs, and this adds none.

**What the compiler refuses.** Both were reproduced on a probe of this layout with the bundled
Go 1.26.5.
- A game package importing a backend package fails with `no required module provides package …`,
  because the game's `go.mod` never requires the backend module.
- If someone added that require anyway, every backend package lives under
  `src/backend/internal/`, so the import still fails with `use of internal package … not allowed`.
- The reverse holds too: the backend never requires the game module, and the game's packages are
  already `internal/`.

**How the modules find each other: `replace`, never `go.work`.**
- These three files each declare `require …/share v0.0.0` plus `replace …/share => ../share`:
  `src/go/go.mod`, `src/go/go.test.mod` and `src/backend/go.mod`. `go.test.mod` already uses the
  same pattern for the ebitestub modules.
- A committed `go.work` is ruled out. In workspace mode `go` refuses the flag
  (`go: -modfile cannot be used in workspace mode`, reproduced), and the game's fast test path
  depends on `-modfile=go.test.mod`.
- The Makefile runs every Go command with `GOWORK=off`. A developer may keep a private `go.work`
  for an editor, but it can never change a build. gopls handles several modules without one.

**What goes in `share`: only code both binaries import.**
- `share/cid`: CIDv1 encoding and parsing, and "address these bytes". The client addresses what it
  uploads. The server recomputes the address and refuses a mismatch.
- `share/wire`: the names and limits both sides must agree on:
  - route paths (`/v1/graphql`, the object routes);
  - header names;
  - the 1 MiB blob cap and the 64 KiB request cap. The client refuses an oversize upload before
    sending it, and the server enforces the same constant.
- `share/api`: the contract, as files rather than running code:
  - `schema/*.graphqls`, moved here from `docs/`;
  - `operations.lock.json`;
  - `schema.go`, which `//go:embed`s both and is imported only by the backend's tests (the lock
    test).

  The game never imports `share/api`. Both generators read its files.

**What stays out of `share`, even though it is about sharing:**
- Client-only code stays in the game (`src/go/internal/grouplog`): log entries, envelopes
  (`crypto/hpke`) and signing. The server never decrypts or verifies them, because clients verify.
  In `share` they would only be dead weight in the server.
- No GraphQL Go type is shared. Each side generates its own from the schema, and the schema is
  the contract.

**Rules for `share`, guarded by `scripts/modguard`.** `make test` and CI run it from its own module.
- `TestModuleRequires` reads all four mod files with `go mod edit -json`:
  - share requires nothing;
  - the game (`go.mod` and `go.test.mod`) requires share and never the backend;
  - the backend requires share and never the game;
  - every `replace` of share points at `../share`;
  - all of them pin the same `toolchain`.
- `TestShareIsPure` runs `go list -deps -json` on share for linux/amd64 and js/wasm:
  - every dependency must be `Standard`;
  - no dependency, direct or transitive, may be `net`, `net/http`, `crypto/tls`, `os/exec` or
    `database/sql`. Share links into the WASM, where `net/http` costs +544 KiB gzip;
  - share's **own** imports (`Imports`, not `Deps`) never include `syscall/js` or `unsafe`. The
    transitive list cannot be used here: a probe CID package (`crypto/sha256` + `encoding/base32`)
    already depends on `unsafe`, and on `syscall/js` under js/wasm, through the standard library;
  - no file may have a `//go:build` line or `import "C"`.
- `TestSharePackagesAreUsedByBoth`: every Go package in share is imported by non-test code in the
  game **and** in the backend. `share/api` is the one exception, with that reason written next to
  it. A package only one side uses moves to that side, so share cannot become a dumping ground.
- In the game, `TestClientBuildsLinkNoBackendDeps` shrinks to the one thing modules cannot catch.
  It becomes `TestWasmLinksNoNetHTTP` (in `internal/deploy`, next to the size budget):
  `net/http` stays out of the js/wasm build, because the desktop build is allowed it.

**Generators.**
- **gqlgen and jet** are `tool` directives in `src/backend/go.mod`, since their runtimes are
  backend dependencies anyway.
  - gqlgen reads `../share/api/schema/*.graphqls`.
  - It inlines a schema file outside its output directory as a string instead of using
    `//go:embed`. In gqlgen v0.17.98 `codegen/data.go`, a path starting with `..` sets
    `embeddable = false`. So the compiled server carries its schema and needs no files at run time.
- **genqlient** is a `tool` in `scripts/apigen/go.mod`, not in the game's `go.mod`.
  - The game's dependency list therefore names no GraphQL module at all.
  - If the import rewrite ever failed, the generated file would not compile. It would not
    silently link genqlient's `net/http`.
  - The rewrite is an **aliased** import, `graphql "…/internal/gqlclient"`. The generated code
    refers to `graphql.Client`, `graphql.Request` and `graphql.Response`, and the probe used only
    those three.
  - Probe: genqlient, run from a separate module against the real schema, generated the client
    into the game tree and exported the operations into `share/api`. The game then built for
    js/wasm with no `net/http` and no genqlient in its `go.mod`, and a malformed query failed
    generation.
  - Finding: genqlient v0.8.1 pins `golang.org/x/tools` v0.24.0, which does not compile under
    Go 1.26 (`tokeninternal.go: invalid array length`). `apigen`'s `go.mod` bumps x/tools;
    v0.52.0 worked.

**Building and testing each module.**
- `make test` stays the game, unchanged.
- `make test-share` runs share's tests on linux and vets it for `GOOS=js`.
- `make test-backend` runs the backend's tests.
- `Dockerfile.api` builds with context `src/`, and `src/.dockerignore` admits only `share/` and
  `backend/`. The game's source is never even sent to the image build.

### Database access layer

**Libraries.**
- **Driver**: `github.com/jackc/pgx/v5` (v5.11), exposed as `*sql.DB` through `pgx/v5/stdlib`.
- **Query builder**: **`github.com/go-jet/jet/v2`** (v2.16). It is type-safe *because* it is
  generated from the migrated schema: one Go type per table and column, so a renamed column or a type
  mismatch is a compile error. Jet runs on `database/sql`; its native pgx support exists only as an
  experimental branch, so the stdlib adapter is used.
- Considered and rejected:
  - sqlc is SQL-first code generation, not a builder;
  - squirrel and goqu are builders that know nothing about the schema;
  - Bob is still pre-1.0.

**Generation is deterministic and checked.** `make gen-db` (Go 1.24+ `tool` directive, `go tool jet`;
the version is pinned in `src/backend/go.mod`):
1. starts the Compose `postgres` service;
2. creates a scratch database and applies the migrations with the API's own `migrate` subcommand;
3. runs `jet -dsn=… -schema=public -path=internal/db/gen`;
4. drops the scratch database.

The output is committed. CI regenerates it on a fresh database and fails on any diff
(`git diff --exit-code`), so the generated code always equals what the migrations produce.

**Layout** (paths are relative to each module's root; see "Code boundaries"):

```
src/backend/  (module …/beatmo/backend)
  cmd/beatmo-api/          main: config from env → wire → serve | migrate | healthcheck
  internal/config          env → typed Config (stdlib; same variable names in every environment)
  internal/db              Open (pgx stdlib, pool sized for Cloud SQL), Tx helper, error mapping
  internal/db/migrations/*.sql   embedded with //go:embed
  internal/db/gen/         jet output (table/, model/), generated, never edited
  internal/store           one file per aggregate: users, sessions, devices, groups, members, envelopes, invites, log, objects, catalog, billing, ratelimit
  internal/authz           Permission constants, the role → permission table, Require
  internal/auth            Caller in the context, argon2id, token issue/verify, Google OIDC
  internal/blob            the GCS JSON API calls (insert, get, delete; get without media = Stat)
  internal/mail            SMTP (github.com/wneessen/go-mail)
  internal/tiers           the tier → limits table, best-of-admins
  internal/billing         Stripe Checkout / portal / webhook parsing (nil when unconfigured)
  internal/api/gqlgen.yml  reads ../../../share/api/schema/*.graphqls
  internal/api/gen/        gqlgen output (executor, model/ = the DTOs), generated, never edited
  internal/api/resolve/    resolvers, one file per schema file
  internal/api             http.go (the plain-HTTP routes), middleware (auth, rate limit, request log)

src/share/  (module …/beatmo/share, standard library only)
  cid/                     CIDv1 encode/parse, hash-and-address
  wire/                    route paths, header names, size caps
  api/schema/*.graphqls    the schema: the contract for server AND client
  api/operations.lock.json what released clients send (append-only)
  api/schema.go            //go:embed of both, imported only by backend tests

src/go/  (the game module, unchanged path)
  internal/gqlclient       Client/Request/Response (from genqlient, MIT) + transport_notjs.go / transport_js.go
  internal/backendclient/operations/*.graphql   what the app sends
  internal/backendclient/genqlient.yaml, generated.go (import rewritten)
  internal/grouplog        log entries, envelopes (crypto/hpke), signing: client-only

scripts/apigen/  (module, generator only)   go.mod with `tool github.com/Khan/genqlient`
scripts/modguard/ (module, guards only)     the boundary tests below
```

**Rules.**
- **Store functions take a `qrm.DB`**, which is satisfied by both `*sql.DB` and `*sql.Tx`, so a
  handler composes several of them in one transaction:
  `db.Tx(ctx, conn, func(q qrm.DB) error { … })`, read committed, rollback on error or panic.
- **Store functions return jet's generated `model.*` structs.** Resolvers map them to gqlgen's
  models, so adding a column (`password_hash`) cannot leak through the API.
- **Store functions never check permissions.** That is `authz.Require`'s job in the resolver, so the
  RBAC matrix tests one layer.
- **Errors are typed.** `qrm.ErrNoRows` → `store.ErrNotFound`; a `pgconn.PgError` with code 23505 →
  `store.ErrConflict{Constraint}`, where the constraint name is one of the explicit names from the
  migrations; anything else is wrapped.
- **No SQL strings outside migrations.** Guard `TestStoreUsesOnlyJet` scans `src/backend/internal` for
  `Exec`/`Query` calls with a string argument. Goose's advisory lock is its own code.
- **Pool**: `MaxOpenConns` 5 per instance. The shared-core Cloud SQL tier allows about 25
  connections and Cloud Run is capped at 3 instances, leaving room for an operator `psql`.

### Migrations

Using **`github.com/pressly/goose/v3`** (v3.29) as a library through its `Provider` API. The API
binary migrates at startup, before it listens, and `beatmo-api migrate` runs the same code for
local work and CI.

1. **SQL only, sequential numbering** (`00001_init.sql`, `00002_…`), with no Go migrations and no
   Down sections. Rolling back means redeploying the previous image, which expand-only keeps safe.
2. **Every statement is idempotent**: re-running a file against the schema it produced is a no-op.
   - `CREATE TABLE IF NOT EXISTS`, `CREATE INDEX IF NOT EXISTS`, `ALTER TABLE … ADD COLUMN IF NOT
     EXISTS`, `CREATE EXTENSION IF NOT EXISTS`.
   - Constraints, which have no `IF NOT EXISTS`, go in a `DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM
     pg_constraint WHERE conname = '…') THEN ALTER TABLE … ADD CONSTRAINT … ; END IF; END $$`.
   - Backfills are written as `UPDATE … WHERE <not yet done>`.
   - No `now()` or `random()` in data a migration writes.
3. **One transaction per file** (goose's default), so a file applies completely or not at all.
   `-- +goose NO TRANSACTION` is allowed only for a file holding exactly one
   `CREATE INDEX CONCURRENTLY IF NOT EXISTS`.
4. **Expand-only**: add, never rename or drop in the same release. A drop ships one release after
   the last code reading the column is gone.
5. **Immutable once on `main`**: `migrations.sum` (SHA-256 per file, like `go.sum`) is committed, and
   `TestMigrationsAreImmutable` fails if an existing file's hash changes or a file is removed. New
   files are appended to the sum.
6. **Serialised across instances** with goose's Postgres session locker (`WithSessionLocker`,
   advisory lock). During a rollout the new revision migrates while the old one keeps serving, and a
   failed migration exits non-zero, so Cloud Run never shifts traffic to it.
7. **Out-of-order migrations are refused** (goose's default).
8. **An older binary must start on a newer schema** (expand-only makes this safe), logging a WARN.
   Goose's behaviour when the database holds versions the binary lacks was not confirmed during
   review, so the builder checks it first.

Tests, run against the Compose Postgres:
- `TestMigrationsAreIdempotent`: for each N, apply files 1..N, take a **schema fingerprint**
  (a catalog query over `information_schema` columns, `pg_indexes` and `pg_constraint`, ordered),
  re-execute file N's SQL directly, and require an identical fingerprint.
- `TestMigrationsAreImmutable`: the `migrations.sum` check above.
- `TestOldBinaryStartsOnNewerSchema`: rule 8.
- `TestGeneratedDBCodeIsCurrent`: the CI diff above, also runnable locally.
- `TestConcurrentStartupMigratesOnce`: start two migrators at once and require exactly one to apply.

Test databases: each package creates its own database from a template that is migrated once
(`CREATE DATABASE t_… TEMPLATE beatmo_migrated`). The DB tests read
`BEATMO_TEST_DATABASE_URL`. When it is unset they **skip loudly** locally and **fail** when
`CI=true`. The fast `-tags test` path never needs Docker.

### Client components

| Piece | Location (proposed) | Notes |
|---|---|---|
| CID | `src/share/cid` (module `share`) | the only piece the server also needs: it re-addresses every upload |
| Log entry, envelope (`crypto/hpke`), signing | `src/go/internal/grouplog` | client-only; pure, standard library, no build tags |
| Group store + object cache | `internal/userprefs` (`share_store*.go`, js/notjs) | file / IndexedDB |
| Backend client | `src/go/internal/backendclient` (genqlient-generated) over `src/go/internal/gqlclient` | see "API › Client"; `net/http` on desktop, `fetch` via `syscall/js` in the browser; every failure is "offline" |
| Blob transfer | `src/go/internal/backendclient` | the plain-HTTP PUT/GET routes, same transports, every response re-hashed |
| Sign-in | `src/go/internal/backendclient` | email/password forms (`TextField`); Google via a browser redirect (web) or a loopback listener on `127.0.0.1` (desktop) |
| Sync | standing pool `share.sync` (1 worker / queue 16) | 3-edit rule; on open, on a timer while focused, on "Sync" |
| Events | `account.signed_in`, `account.signed_out`, `group.joined`, `group.left`, `share.sent`, `share.received`, `catalog.synced` | tags `account`, `share` (new, lowercase) |
| UI | sign-in sheet, Groups menu, group members (role pills, remove, admin-only), invite sheet (role choice), inbox, publish dialog, catalog browser; every action shown or hidden from `myPermissions` | scenes `sign_in`, `groups_menu`, `group_members_admin`, `group_members_reader`, `group_invite`, `share_inbox`, `publish_dialog`, `catalog_browser` (+ mobile) |
| Service worker | `src/js/sw.js` | see "Constraints" |
| WASM bridge | new exports added to `wasm_bridge_smoke.browser.test.js` | |

## Code shape: main packages, types and functions

These are the proposed signatures, enough to plan from; they are not final. Generated names
(`model.*` from jet, `gen.*` / `genmodel.*` from gqlgen, `backendclient.*` from genqlient) follow
their generators. `ctx` is a `context.Context` and `q` is a jet `qrm.DB` (satisfied by `*sql.DB` and
`*sql.Tx`). Ids are `uuid.UUID` on the server and `string` in the client.

**How one request flows** (`shareObject`):

```
POST /v1/graphql
  api.Server.withCaller           Bearer token → auth.Issuer.VerifyAccess → auth.WithCaller(ctx)
  gqlgen executor                 parse (LRU), complexity ≤ 300, AroundOperations: rate limit + log line
  resolve.mutationResolver.ShareObject
    db.Tx(ctx, conn, func(q) {
      store.LockGroup(ctx, q, groupID)                         SELECT … FOR UPDATE
      authz.Require(ctx, q, groupID, authz.ContentWrite)      one primary-key read
      blobs.Stat(ctx, blob.Group, blob.GroupName(groupID, cid))     → OBJECT_MISSING
      store.GroupStorageUsed + tiers.ForGroup                 → QUOTA_EXCEEDED
      store.AddGroupObject(...)
      store.AppendLog(ctx, q, groupID, store.LogAppend{Kind: share, ...})   log_seq + 1
    })
  map model.GroupLog → genmodel.LogEntry
```

### Backend (`src/backend`)

**`cmd/beatmo-api`**: `main` reads `config.Load(os.Getenv)`, then runs one subcommand:
- `serve` (the default) migrates, wires the dependencies and listens on `$PORT`;
- `migrate` migrates and exits;
- `healthcheck` GETs `/v1/healthz` and exits 0 or 1 (Compose).

**`internal/config`**
```go
type Config struct {
    Port          int      // $PORT (Cloud Run sets it; 8080 locally)
    DatabaseURL   string
    PublicBaseURL *url.URL
    GCS           GCS      // Endpoint (empty = Google), PublicBucket, GroupBucket
    SMTPURL       string
    MailFrom      string
    OIDC          OIDC     // Issuer, ClientID, ClientSecret
    TokenKeys     []Key    // Key{KID string; Secret []byte}; first signs, all verify
    Stripe        Stripe   // SecretKey, WebhookSecret, PriceID; all empty = billing off
    Playground    bool
}
func Load(getenv func(string) string) (Config, error) // every missing or malformed variable in one error
```

**`internal/db`**
```go
func Open(ctx context.Context, url string) (*sql.DB, error)            // pgx stdlib, MaxOpenConns 5, ping
func Tx(ctx context.Context, conn *sql.DB, fn func(q qrm.DB) error) error // read committed; rollback on error or panic
func Migrate(ctx context.Context, conn *sql.DB) error                  // goose Provider + session locker over migrations.FS
```
`internal/db/migrations` exports `FS embed.FS`. `internal/db/gen` is jet's output:
- `gen/beatmo/public/model`: one struct per table, for example
  `model.GroupMembers{GroupID, UserID uuid.UUID; Role model.GroupRole; JoinedAt time.Time; …}`,
  plus enum types (`model.GroupRole_Admin`);
- `gen/beatmo/public/table`: the column builders (`table.GroupMembers.Role.EQ(…)`).

**`internal/store`**: plain functions, one file per aggregate. They return `model.*` and never
check permissions.
```go
var ErrNotFound = errors.New("store: not found")
type ErrConflict struct{ Constraint string } // a unique violation, by its explicit constraint name

// users.go
func CreateUser(ctx, q, email, displayName, passwordHash string) (model.Users, error)
func UserByID(ctx, q, id uuid.UUID) (model.Users, error)
func UserByEmail(ctx, q, email string) (model.Users, error)
func SetPassword(ctx, q, userID uuid.UUID, hash string) error
func MarkVerified(ctx, q, userID uuid.UUID, at time.Time) error
func LinkOrCreateGoogleUser(ctx, q, id GoogleIdentity) (model.Users, error) // the four linking rules, in order
func DeleteUser(ctx, q, userID uuid.UUID) error  // after the promotions; cascades devices, sessions, envelopes
type GoogleIdentity struct{ Sub, Email, Name string }

// devices.go, sessions.go
type DeviceKeys struct{ Name string; SignPub, BoxPub []byte }
func UpsertDevice(ctx, q, userID uuid.UUID, d DeviceKeys) (model.Devices, error) // by (user_id, sign_pub)
func DevicesOf(ctx, q, userID uuid.UUID) ([]model.Devices, error)
func DeleteDevice(ctx, q, userID, deviceID uuid.UUID) error
func CreateSession(ctx, q, s model.Sessions) (model.Sessions, error)
func SessionByRefresh(ctx, q, hash []byte) (s model.Sessions, rotatedOut bool, err error) // matches refresh_hash or prev_refresh_hash
func RotateSession(ctx, q, id uuid.UUID, newHash []byte, expires time.Time) error   // prev ← current, current ← new
func DeleteSession(ctx, q, id uuid.UUID) error
func DeleteUserSessions(ctx, q, userID uuid.UUID) error

// groups.go, members.go, envelopes.go
func CreateGroup(ctx, q, name string, creator uuid.UUID) (model.Groups, error) // + the admin membership row
func LockGroup(ctx, q, id uuid.UUID) (model.Groups, error)                     // SELECT … FOR UPDATE
func RenameGroup(ctx, q, id uuid.UUID, name string) (model.Groups, error)
func DeleteGroup(ctx, q, id uuid.UUID) error
func Membership(ctx, q, groupID, userID uuid.UUID) (model.GroupMembers, error)
func MembershipsOf(ctx, q, userID uuid.UUID) ([]MembershipRow, error)  // + the group row
func Members(ctx, q, groupID uuid.UUID) ([]MemberRow, error)           // one join: members + users + devices
func AddMember(ctx, q, m model.GroupMembers) error
func SetRole(ctx, q, groupID, userID uuid.UUID, role model.GroupRole, by uuid.UUID, at time.Time) error
func RemoveMember(ctx, q, groupID, userID uuid.UUID) error
func CountAdmins(ctx, q, groupID uuid.UUID) (int, error)
func CountMembers(ctx, q, groupID uuid.UUID) (int, error)
func SuccessorAdmin(ctx, q, groupID uuid.UUID) (uuid.UUID, bool, error) // earliest-joined writer, else reader
type MemberRow struct{ Member model.GroupMembers; User model.Users; Devices []model.Devices }
type MembershipRow struct{ Member model.GroupMembers; Group model.Groups }
func PutEnvelopes(ctx, q, groupID uuid.UUID, env []model.GroupEnvelopes) error  // ErrConflict when one exists
func Envelope(ctx, q, groupID, deviceID uuid.UUID) (model.GroupEnvelopes, error)
func DevicesAwaitingKey(ctx, q, groupID uuid.UUID) ([]MemberDeviceRow, error)   // members' devices anti-join envelopes

// invites.go
func CreateInvite(ctx, q, inv model.Invites) (model.Invites, error)
func InviteByCodeHash(ctx, q, hash []byte) (model.Invites, error)
func RedeemInvite(ctx, q, id, userID uuid.UUID, at time.Time) error
func RevokeInvite(ctx, q, id uuid.UUID, at time.Time) (model.Invites, error)  // returns it so the resolver can check its group
func PendingInvites(ctx, q, groupID uuid.UUID, now time.Time) ([]model.Invites, error)

// log.go, objects.go
type LogAppend struct {
    Kind         model.LogKind
    Object       *string   // CID text; nil for device_added
    AuthorUser   uuid.UUID
    AuthorDevice uuid.UUID
    Entry        []byte    // the sealed document, opaque to the server
    At           time.Time
}
func AppendLog(ctx, q, groupID uuid.UUID, e LogAppend) (model.GroupLog, error) // log_seq + 1 under the group lock
func LogAfter(ctx, q, groupID uuid.UUID, after int64, limit int) ([]model.GroupLog, error)
func AddGroupObject(ctx, q, o model.GroupObjects) error
func GroupObject(ctx, q, groupID uuid.UUID, cid string) (model.GroupObjects, error)
func DeleteGroupObject(ctx, q, groupID uuid.UUID, cid string) error
func GroupObjects(ctx, q, groupID uuid.UUID) ([]model.GroupObjects, error)
func GroupStorageUsed(ctx, q, groupID uuid.UUID) (int64, error)
func SharesBy(ctx, q, userID uuid.UUID) ([]model.GroupObjects, error)

// catalog.go, billing.go, ratelimit.go
func Publish(ctx, q, o model.PublicObjects) (model.PublicObjects, error)  // same (cid, publisher) again = the existing row
func Delist(ctx, q, cid string, publisher uuid.UUID) (lastListing bool, err error) // true: delete the blob after commit
func Catalog(ctx, q) (entries []model.PublicObjects, etag string, err error)
func CatalogETag(ctx, q) (string, error)                                  // the cheap query, for notModified
func PublicStorageUsed(ctx, q, userID uuid.UUID) (int64, error)
func Entitlement(ctx, q, userID uuid.UUID) (model.Entitlements, error)   // ErrNotFound = free
func UpsertEntitlement(ctx, q, e model.Entitlements) error
func AdminTiers(ctx, q, groupID uuid.UUID) ([]model.Tier, error)
func Hit(ctx, q, key string, window time.Time) (hits int, err error)
func PurgeRateLimits(ctx, q, before time.Time) error
```

**`internal/authz`**: the only place roles are named.
```go
type Permission uint8
const (
    GroupRead Permission = iota + 1
    KeyDistribute
    ContentWrite
    ContentModerate
    MembersInvite
    MembersManage
    GroupManage
)
type PermSet uint16
func (s PermSet) Has(p Permission) bool
func (s PermSet) List() []Permission                       // sorted, for Group.myPermissions
var rolePermissions = map[model.GroupRole]PermSet{ … }     // the table in "Groups"
func Of(role model.GroupRole) PermSet

type Grant struct{ Member model.GroupMembers; Perms PermSet }
var (
    ErrNotMember = errors.New("authz: not a member") // → NOT_FOUND
    ErrForbidden = errors.New("authz: forbidden")    // → FORBIDDEN
)
// Require reads the caller (auth.CallerFrom) and their membership row inside the caller's transaction.
func Require(ctx context.Context, q qrm.DB, groupID uuid.UUID, p Permission) (Grant, error)
```

**`internal/auth`**
```go
type Caller struct{ UserID, SessionID, DeviceID uuid.UUID }
func WithCaller(ctx context.Context, c Caller) context.Context
func CallerFrom(ctx context.Context) (Caller, bool)

// password.go: argon2id, PHC string format
func HashPassword(pw string) (string, error)
func CheckPassword(phc, pw string) (bool, error)

// tokens.go: go-jose, HKDF subkey per purpose
type Issuer struct{ /* keys, now func() time.Time */ }
func NewIssuer(keys []config.Key, now func() time.Time) (*Issuer, error)
func (i *Issuer) Access(c Caller) (token string, exp time.Time, err error)       // JWT, 15 min
func (i *Issuer) VerifyAccess(token string) (Caller, error)
func (i *Issuer) EmailToken(userID uuid.UUID) (string, error)
func (i *Issuer) VerifyEmailToken(tok string) (uuid.UUID, error)
func (i *Issuer) ResetToken(userID uuid.UUID, passwordHash string) (string, error) // carries the hash fingerprint
func (i *Issuer) VerifyResetToken(tok string, currentHash func(uuid.UUID) (string, error)) (uuid.UUID, error)
func (i *Issuer) Handoff(userID uuid.UUID, challenge string) (string, error)     // 60 s
func (i *Issuer) VerifyHandoff(tok, verifier string) (uuid.UUID, error)          // sha256(verifier) == challenge
func (i *Issuer) SealState(s OIDCState) (string, error)                          // JWE for the state cookie
func (i *Issuer) OpenState(s string) (OIDCState, error)
func NewRefreshToken() (token string, hash []byte)

// google.go: go-oidc + oauth2, PKCE
type Google struct{ /* provider, oauth2.Config, verifier, issuer */ }
func NewGoogle(ctx context.Context, cfg config.OIDC, base *url.URL, iss *Issuer) (*Google, error)
func (g *Google) StartURL(challenge, ret string) (redirect string, stateCookie *http.Cookie, err error)
func (g *Google) Finish(ctx context.Context, r *http.Request) (store.GoogleIdentity, OIDCState, error)
```

**`internal/blob`**: the GCS JSON API over HTTPS, no SDK.
```go
type Bucket int // Public, Group
type Store struct{ /* *http.Client from oauth2/google, endpoint, bucket names */ }
func New(ctx context.Context, cfg config.GCS) (*Store, error)  // no auth when Endpoint is set (fake-gcs-server)
func (s *Store) Put(ctx context.Context, b Bucket, name string, data []byte, contentType string) (created bool, err error) // ifGenerationMatch=0
func (s *Store) Get(ctx context.Context, b Bucket, name string) ([]byte, error)
func (s *Store) Stat(ctx context.Context, b Bucket, name string) (size int64, err error) // same objects.get call, metadata only
func (s *Store) Delete(ctx context.Context, b Bucket, name string) error
func PublicName(c cid.CID) string                // "ipfs/<cid>"
func GroupName(groupID uuid.UUID, c cid.CID) string // "g/<group>/<cid>"
```

**`internal/mail`**
```go
type Message struct{ To, Subject, Text string }
type Sender interface{ Send(ctx context.Context, m Message) error }
func NewSMTP(url, from string) (Sender, error)                   // go-mail
func Verify(to string, link *url.URL) Message
func AlreadyRegistered(to string) Message
func Reset(to string, link *url.URL) Message
```
Mail is sent **after** the transaction commits. A failed send is logged; the mutation still
succeeds, since `register` and `forgotPassword` must not reveal anything anyway.

**`internal/tiers`** and **`internal/billing`**
```go
type Limits struct{ MemberCap int; GroupStorage, PublicStorage int64 }
var Table = map[model.Tier]Limits{ … }          // the placeholders in "Tiers"; also Query.tiers
func Best(ts []model.Tier) model.Tier           // by the table, never by label order
func ForUser(ctx, q, userID uuid.UUID) (model.Tier, Limits, error)
func ForGroup(ctx, q, groupID uuid.UUID) (model.Tier, Limits, error) // best tier among its admins

type Client struct{ /* stripe-go client, price id, return URLs */ }   // nil when Stripe is unset
func New(cfg config.Stripe, base *url.URL) *Client
func (c *Client) CheckoutURL(ctx context.Context, u model.Users, customerID string) (string, error)
func (c *Client) PortalURL(ctx context.Context, customerID string) (string, error)
func (c *Client) ParseWebhook(payload []byte, sig string) (Update, error) // webhook.ConstructEvent
type Update struct{ CustomerID string; Tier model.Tier; PeriodEnd time.Time }
```

**`internal/api`** (HTTP) and **`internal/api/resolve`** (GraphQL)
```go
type Deps struct {
    DB      *sql.DB
    Issuer  *auth.Issuer
    Google  *auth.Google
    Blobs   *blob.Store
    Mail    mail.Sender
    Billing *billing.Client
    Config  config.Config
    Now     func() time.Time
}
func NewServer(d Deps) *Server
func (s *Server) Handler() http.Handler // ServeMux: /v1/graphql, auth routes, object routes, webhook, healthz
// http.go: googleStart, googleCallback, putGroupObject, getGroupObject, putPublicObject, webhook, healthz
// middleware.go: withCaller, withRequestID, maxBytes(wire.MaxRequestBytes)

// resolve/: gqlgen's follow-schema layout, one *.resolvers.go per schema file
// api imports resolve to build the handler, so the resolver carries its own fields (no import cycle)
type Resolver struct {
    DB      *sql.DB
    Issuer  *auth.Issuer
    Blobs   *blob.Store
    Mail    mail.Sender
    Billing *billing.Client
    Base    *url.URL
    Now     func() time.Time
}
func (r *Resolver) Query() gen.QueryResolver
func (r *Resolver) Mutation() gen.MutationResolver
func (r *Resolver) Group() gen.GroupResolver        // members, limits, myEnvelope, log, invites, …
func (r *Resolver) Me() gen.MeResolver              // devices, memberships, publications, shares
// e.g. func (r *mutationResolver) ShareObject(ctx context.Context, in genmodel.ShareObjectInput) (*genmodel.LogEntryPayload, error)
// map.go: toLogEntry(model.GroupLog) *genmodel.LogEntry, toMember(store.MemberRow) *genmodel.Member, …
// errors.go: userError(code, field), presenter (store/authz errors → extensions.code)
```
The schema splits by area: `common.graphqls` (scalars, enums, `UserError`, payloads), `account`,
`groups`, `catalog` and `billing`, each adding its fields with `extend type Query` / `extend type
Mutation`. gqlgen then generates one resolver file per area.

### Share (`src/share`)

```go
// cid
type CID struct{ /* sha2-256 digest */ }
func Of(data []byte) CID                       // address these bytes
func Parse(s string) (CID, error)              // CIDv1 raw sha2-256 base32 only
func FromDigest(d [32]byte) CID                // from Beatmo's existing SHA-256 hashes
func (c CID) String() string                   // "bafkrei…"
func (c CID) Digest() [32]byte
func (c CID) Verify(data []byte) bool
func (c CID) MarshalText() ([]byte, error)
func (c *CID) UnmarshalText(b []byte) error

// wire
const (
    GraphQLPath         = "/v1/graphql"
    GroupObjectPattern  = "/v1/groups/{id}/objects/{cid}"
    PublicObjectPattern = "/v1/objects/{cid}"
    GoogleStartPath     = "/v1/auth/google/start"
    GoogleCallbackPath  = "/v1/auth/google/callback"
    HealthPath          = "/v1/healthz"
    IPFSPrefix          = "/ipfs/"
    MaxObjectBytes      = 1 << 20
    MaxRequestBytes     = 64 << 10
    MaxLogEntryBytes    = 16 << 10
    HeaderRequestID     = "X-Request-Id"
    ContentTypeRaw      = "application/vnd.ipld.raw"
)
func GroupObjectPath(groupID, cid string) string
func PublicObjectPath(cid string) string

// api (imported only by backend tests)
var Schema embed.FS           // schema/*.graphqls
var OperationsLock []byte     // operations.lock.json
```

### Game (`src/go`)

**`internal/gqlclient`**: the three types genqlient's generated code uses, plus the transports.
```go
type Client interface {
    MakeRequest(ctx context.Context, req *Request, resp *Response) error
}
type Request struct {
    Query     string `json:"query"`
    Variables any    `json:"variables,omitempty"`
    OpName    string `json:"operationName"`
}
type Response struct {
    Data       any            `json:"data"`
    Extensions map[string]any `json:"extensions,omitempty"`
    Errors     ErrorList      `json:"errors,omitempty"` // our type: genqlient's uses gqlparser
}
type Error struct {
    Message    string
    Path       []any
    Code       string // extensions.code
    RetryAfter int    // extensions.retryAfterSeconds
    RequestID  string
}
type ErrorList []Error
func (l ErrorList) Error() string
func (l ErrorList) Code() string               // the first error's code
var ErrOffline = errors.New("backend offline") // network error, 5xx or timeout

type HTTPClient struct{ /* endpoint, token func, timeout */ }
func New(endpoint string, token func(context.Context) (string, error)) *HTTPClient
func (c *HTTPClient) MakeRequest(ctx context.Context, req *Request, resp *Response) error

// transport_notjs.go (net/http) and transport_js.go (fetch), same signature:
type RawRequest struct{ Method, URL string; Header map[string]string; Body []byte }
type RawResponse struct{ Status int; Header map[string]string; Body []byte }
func Do(ctx context.Context, r RawRequest) (RawResponse, error) // 15 s timeout; failure → ErrOffline
```

**`internal/backendclient`**
```go
// generated.go (genqlient): one function per operation in operations/*.graphql, for example
func Login(ctx context.Context, client graphql.Client, input LoginInput) (*LoginResponse, error)
func SyncGroup(ctx context.Context, client graphql.Client, id string, after int64) (*SyncGroupResponse, error)
func ShareObject(ctx context.Context, client graphql.Client, input ShareObjectInput) (*ShareObjectResponse, error)
// plus their input/response structs and the enums (GroupRole, Permission, ErrorCode, …)

// backend.go (hand-written)
type Tokens struct{ Access, Refresh string; AccessExpires, RefreshExpires time.Time }
type TokenStore interface{ Load() (Tokens, error); Save(Tokens) error; Clear() error } // file / IndexedDB
type Backend struct{ /* base URL, *gqlclient.HTTPClient, TokenStore, refresh single-flight */ }
func New(baseURL string, tokens TokenStore) *Backend
func (b *Backend) GQL() graphql.Client // adds the bearer token; on UNAUTHENTICATED refreshes once and retries
func (b *Backend) PutGroupObject(ctx context.Context, groupID string, data []byte) (cid.CID, error)
func (b *Backend) GetGroupObject(ctx context.Context, groupID string, c cid.CID) ([]byte, error) // re-hashed
func (b *Backend) PutPublicObject(ctx context.Context, data []byte) (cid.CID, error)
func (b *Backend) FetchPublic(ctx context.Context, c cid.CID) ([]byte, error) // beatmo.io/ipfs, then gateways
func (b *Backend) GoogleStartURL(ret string) (url string, verifier string, err error)
```
In the browser, the refresh single-flight wraps `navigator.locks.request("beatmo-refresh", …)`,
reached through `syscall/js` behind the same js/notjs pair.

**`internal/grouplog`**: client-only crypto, standard library only.
```go
type DeviceKeys struct{ Sign ed25519.PrivateKey; Box *ecdh.PrivateKey }
func NewDeviceKeys() (DeviceKeys, error)
func (k DeviceKeys) Public() (signPub, boxPub []byte)

type GroupKey [32]byte
func NewGroupKey() (GroupKey, error)
func WrapKey(k GroupKey, recipientBoxPub []byte, groupID string, epoch int) ([]byte, error)  // HPKE seal
func UnwrapKey(sealed []byte, dev DeviceKeys, groupID string, epoch int) (GroupKey, error)
func SealObject(k GroupKey, groupID string, epoch int, plain []byte) ([]byte, error)          // AES-256-GCM, AD = group|epoch
func OpenObject(k GroupKey, groupID string, epoch int, sealed []byte) ([]byte, error)

type Kind int // Share, Unshare, DeviceAdded over an enum.Names table; an unknown spelling decodes to KindUnknown (kept, not acted on)
type Meta struct{ Kind, Title string; Size int }
type Entry struct {
    schema.Envelope
    Group         string    `json:"group"`
    Epoch         int       `json:"epoch"`
    AuthorDevice  string    `json:"author_device"`
    AuthorSignPub []byte    `json:"author_sign_pub"`
    Kind          Kind      `json:"kind"`
    Object        string    `json:"object,omitempty"` // CID text
    Devices       []string  `json:"devices,omitempty"` // device_added: the devices given a key
    Meta          *Meta     `json:"meta,omitempty"`
    CreatedAt     time.Time `json:"created_at"`
}
func (e Entry) SignAndSeal(dev DeviceKeys, k GroupKey) ([]byte, error)
// OpenEntry decrypts and verifies the signature, then refuses the entry when its kind or object
// differ from the server's plaintext copies.
func OpenEntry(k GroupKey, groupID string, sealed []byte, plainKind Kind, plainObject string) (Entry, error)
func (e Entry) ID() [32]byte // hash of the signed document: the set-union identity
```

## Infrastructure

### Shape (production)

| Component | Resource | Notes |
|---|---|---|
| API | Cloud Run service `beatmo-api`, us-central1 | image by digest; 512 MiB, 1 vCPU, CPU only during requests, min 0 / max 3 instances, concurrency 80; **ingress `internal-and-cloud-load-balancing`** so the `run.app` URL cannot bypass the load balancer; startup probe `/v1/healthz` (it passes only after migrations) |
| Service identity | service account `beatmo-api` | `cloudsql.client`; `secretAccessor` on each of its secrets; `storage.objectAdmin` on the two object buckets only |
| Front door | existing global HTTPS LB | add a serverless NEG + backend service (no CDN) for `/v1/*`, a backend bucket (CDN, the response headers above) for `/ipfs/*`, path rules in `www-beatmo-io-static-lb`; **fix**: give port 80 its own URL map that redirects to HTTPS (today it serves the site in clear), and add the missing IPv6 HTTPS forwarding rule (only port 80 has one) |
| Database | Cloud SQL for PostgreSQL 17, Enterprise, shared-core `db-f1-micro`, 10 GB SSD | public IP with **no authorized networks**; reached only through Cloud Run's built-in Cloud SQL connection (unix socket `/cloudsql/<conn>`, encrypted, IAM-checked), so no VPC or peering; SSL required; daily backups + 7-day point-in-time recovery; deletion protection on |
| Objects | GCS `beatmo-public-objects` (public read, uniform access), `beatmo-group-objects` (private) | us-central1, default soft delete |
| Secrets | Secret Manager | `DB_PASSWORD`, `TOKEN_KEYS`, `SMTP_PASSWORD`, `GOOGLE_CLIENT_SECRET`, `STRIPE_SECRET_KEY`, `STRIPE_WEBHOOK_SECRET`; injected by Cloud Run as env vars (`--set-secrets`), so the API never calls Secret Manager |
| Images | Artifact Registry Docker repo | cleanup policy keeps the last 10 |
| DNS | **Cloud DNS** zone `beatmo-io` (moved from Hostinger, see below) | apex A + AAAA → the LB addresses, `www` CNAME, the email provider's SPF/DKIM/DMARC records, CAA for `pki.goog` |
| Monitoring | uptime check on `https://beatmo.io/v1/healthz`, email alert; billing budget alerts 50/90/100 % | |
| CI identity | Workload Identity Federation pool + provider for GitHub Actions, deploy service account | no key files |

The API image is a multi-stage build with the repo's bundled Go version and `CGO_ENABLED=0`, on
`gcr.io/distroless/static-debian12:nonroot` pinned by digest. Distroless has no shell or curl, so the
binary carries a `healthcheck` subcommand for Compose.

**DNS move.** Hostinger hosts the zone today (`ns1/ns2.dns-parking.com`). It has two records (apex A
→ `34.149.160.89`, `www` CNAME), no DNSSEC and no mail records, so the move is low-risk:
1. `provision.sh` creates the Cloud DNS zone and every record.
2. Verify with `dig @ns-cloud-…`.
3. Change the nameservers at Hostinger (the one manual step).

The existing certificate keeps working because the A record's value does not change. From then on
every record is in `backend.env` / `provision.sh`.

### Configuration: one set of variable names everywhere

The API reads only environment variables. Production gets them from Cloud Run (plain values from
`backend.env`, secrets from Secret Manager); Compose gets them from `deploy/backend/local.env`
(committed, dev-only values). The names never differ:

| Variable | Production | Local (Compose) |
|---|---|---|
| `DATABASE_URL` | `host=/cloudsql/beatmo:us-central1:beatmo-pg dbname=beatmo user=beatmo_api` + `DB_PASSWORD` | `postgres://beatmo:beatmo@postgres:5432/beatmo?sslmode=disable` |
| `PUBLIC_BASE_URL` | `https://beatmo.io` | `http://localhost:8080` |
| `GCS_ENDPOINT` | unset (→ `https://storage.googleapis.com`) | `http://gcs:4443` (no auth) |
| `GCS_PUBLIC_BUCKET` / `GCS_GROUP_BUCKET` | `beatmo-public-objects` / `beatmo-group-objects` | same names |
| `SMTP_URL`, `MAIL_FROM` | provider's submission endpoint, port 465/587 (Google Cloud blocks 25) | `smtp://mail:1025` |
| `OIDC_ISSUER`, `OIDC_CLIENT_ID`, `GOOGLE_CLIENT_SECRET` | `https://accounts.google.com`, the web client | `http://oidc.localhost:8091/google`, `local`, `local` |
| `TOKEN_KEYS` | secret | fixed dev key |
| `STRIPE_*` | secrets | unset → billing off (or test-mode keys + `stripe listen`) |
| `GRAPHQL_PLAYGROUND` | unset | `1` (GraphiQL at `/v1/graphql/play`) |

### Local stack (`deploy/backend/compose.yaml`)

Each service stands in for exactly one production component, so `docker compose up --wait` runs the
whole system on `http://localhost:8080`:

| Service | Image (pinned by digest) | Mirrors |
|---|---|---|
| `proxy` | `caddy:2.11` | the load balancer's URL map: `/v1/*` → `api` (round robin over both replicas), `/ipfs/*` → `gcs` (rewritten to `/beatmo-public-objects/ipfs/…`, with the same response headers as the backend bucket), everything else → the site (`src/js`, or `SITE_DIR=build/wasm_site` for the exact deploy manifest) |
| `api` ×2 | built from `Dockerfile.api` (the production image) | Cloud Run: `read_only: true`, non-root, `local.env`, healthcheck `beatmo-api healthcheck`; both replicas migrate at start, which exercises the advisory lock |
| `postgres` | `postgres:17` (the Cloud SQL major) | Cloud SQL; named volume |
| `gcs` | `fsouza/fake-gcs-server:1.56` | Cloud Storage; filesystem backend, buckets created from seed directories |
| `mail` | `axllent/mailpit:v1.32` | the SMTP provider; inbox UI on `:8025`, and its HTTP API lets tests read the verification mail |
| `oidc` | `ghcr.io/navikt/mock-oauth2-server:6.1` | accounts.google.com: any `sub`/email on its login form. Published on host port 8091 and aliased `oidc.localhost` in the Compose network, so the browser (browsers resolve `*.localhost` to loopback) and the API container see the same issuer URL. The builder verifies the container side resolves the alias through Docker DNS |

Not mirrored: the CDN (Caddy serves without caching), Cloud Run's scale-to-zero, and Google's real
consent screen. Real Google sign-in works locally too, by pointing `OIDC_*` at a real client with the
`http://localhost:8080/v1/auth/google/callback` redirect.

Make targets:
- `make backend-up` / `backend-down`;
- `make gen-db`, `make gen-api` (gqlgen + genqlient + import rewrite + operations lock);
- `make test-backend`: brings up `postgres` and runs the store and migration tests;
- `make backend-e2e`: the full stack. One Go test drives it with the **generated client**, the same
  code the app ships, over plain HTTP for the blobs:
  1. register → read the mail from Mailpit → verify → log in;
  2. create a group → invite as reader → a second user joins;
  3. the reader's blob PUT is `FORBIDDEN`;
  4. the admin promotes them to writer → they share → the first user syncs the log and fetches the
     blob;
  5. the admin demotes them again → their next share is `FORBIDDEN`, with no new token needed;
  6. the only admin's `leaveGroup` returns `LAST_ADMIN`;
  7. log out.

  Google sign-in goes through the mock's form.

### Files (`deploy/backend/`)

| File | Purpose |
|---|---|
| `backend.env` | committed, non-secret source of truth; every script takes the env file as its first argument (a `staging.env` needs no script change): project, region, names, Cloud Run limits, Cloud SQL tier + PG major, buckets, DNS records, budget ceiling, the image digest deployed |
| `provision.sh` | idempotent describe-or-create of everything in "Shape" (the `resource_exists` style of `setup-gcs-static-site.sh`), plus in-place patches of the cheap fields (Cloud SQL tier, Cloud Run limits, DNS records, budget); anything else it reports and stops |
| `check.sh` | read-only drift report against `backend.env` |
| `deploy.sh <digest>` | `gcloud run deploy` with the full flag set from `backend.env`; waits until the revision is ready (it migrated); on failure Cloud Run keeps the previous revision; records the digest in `backend.env`. Rollback: `deploy.sh <previous digest>` or `gcloud run services update-traffic --to-revisions=<prev>=100` |
| `gc-orphans.sh` | list/remove blobs no table references |
| `takedown.sh <cid>` | operator removal: every listing of the CID, its blob, and a CDN invalidation |
| `manual-steps.md` | the steps `gcloud` cannot do: nameserver change at Hostinger; the OAuth consent screen and web OAuth client (console only); the SMTP provider account and domain verification; the Stripe product/price and webhook endpoint; `gcloud secrets versions add` for each secret value |
| `compose.yaml`, `local.env`, `Caddyfile`, `gcs-seed/` | the local stack |
| `Dockerfile.api` | the API image |

### Releases and CI

- The existing `ci.yml` gets a `backend` job:
  1. `docker compose up -d --wait postgres`;
  2. backend tests;
  3. `make gen-db` + `make gen-api` + `git diff --exit-code`;
  4. `scripts/modguard` (module boundaries), `TestReleasedClientOperationsStillValidate`;
  5. build the image.
- On `main`: push the image tagged with the git SHA, then `deploy.sh <digest>`, authenticated with
  Workload Identity Federation.
- `backend-e2e` runs in CI on the same Compose file.

### Dependencies (complete list)

API: Go standard library plus:

| Module | For | Status (2026-10) |
|---|---|---|
| `github.com/jackc/pgx/v5` | Postgres driver | 14k ★, v5.11 (2026-09) |
| `github.com/go-jet/jet/v2` | type-safe query builder + generator | 3.8k ★, v2.16 (2026-10) |
| `github.com/pressly/goose/v3` | migrations | 11.6k ★, v3.29 (2026-10) |
| `golang.org/x/oauth2` (+ `/google`) | OAuth code flow, PKCE, GCS tokens | Go project |
| `github.com/coreos/go-oidc/v3` (brings `go-jose/v4`) | ID token verification; our JWS/JWE | 2.5k ★, v3.21 (2026-09) |
| `golang.org/x/crypto/argon2` | password hashing | Go project |
| `github.com/wneessen/go-mail` | SMTP | 1.5k ★, v0.8 (2026-07) |
| `github.com/stripe/stripe-go` | billing | official, v87 (2026-10) |
| `github.com/99designs/gqlgen` | GraphQL server: generator + runtime | v0.17.98 (2026-10-22), releases weekly |

Generators (`tool` directives: jet in `src/backend/go.mod`; genqlient in `scripts/apigen/go.mod`, so the game's
`go.mod` lists no GraphQL module):
- jet's `cmd/jet`;
- `github.com/Khan/genqlient` v0.8.1 (2025-05). Its generated code's single runtime import is
  rewritten to `src/go/internal/gqlclient`.

Plus what these bring with them:
- `google/uuid` for jet's uuid columns;
- `lib/pq` for jet's generator only, never linked into the API;
- gqlgen's `vektah/gqlparser/v2`, `hashicorp/golang-lru/v2`, and `gorilla/websocket` (linked
  through gqlgen's transport package but never routed).

Client: Go standard library, plus `src/go/internal/gqlclient`'s copy of genqlient's types. Infra: `gcloud`, Docker.
Local only: Caddy, fake-gcs-server, Mailpit, mock-oauth2-server, Postgres images.

### Rough monthly cost (list prices, us-central1; confirm with the pricing calculator)

| Item | ≈ / month |
|---|---|
| Cloud SQL `db-f1-micro` + 10 GB SSD + backups | $10–12 |
| Cloud Run (scale to zero; small traffic stays in the free tier) | ~$0 |
| Cloud Storage + CDN egress | < $1 at small scale; usage-dependent |
| Cloud DNS zone | $0.20 |
| Artifact Registry, Secret Manager, uptime check | < $1 |
| Load balancer | ~$0 extra (forwarding rules already paid for by the site) |
| **Total added** | **≈ $12–15** |

Known trade-offs: shared-core Cloud SQL has no SLA (a dedicated core is ~$50/month, a tier change in
`backend.env`); no database high availability (recovery from backups/PITR); Cloud Run cold starts
add up to ~1 s to the first request after idle (`min-instances 1` removes it for ~$5–10/month).

## Phases

1. **This document.**
2. **Direct transport + hardening.** The desktop builds get peer-to-peer transfer (`tsnet` + a
   coordination server, or an IPFS node). That needs a long-lived process with state, so it revisits
   hosting: Cloud Run does not fit it. With content leaving server-gated channels, group key rotation
   arrives here (`epoch` is already in the format), together with approving new devices from an
   existing one.
3. **Premium live co-editing**: CRDT documents for circuits and instruments, carried as new log entry
   kinds over the phase 2 channel, and the log for offline catch-up.

## Left to the builder

- Rate-limit numbers; extra `ErrorCode` values if a refusal needs its own UI text.
- Operation names and selections in `operations/*.graphql` (the schema fixes what is possible).
- Which further genqlient `graphql` identifiers the generated code references, if more than
  `Client`/`Request`/`Response` (copy them into `gqlclient`).
- Sync timer interval and backoff.
- UI layout of the new surfaces, within the MenuCard / TextField / scene rules.
- The checks marked "the builder verifies" above: goose's behaviour on a newer schema, and the
  `oidc.localhost` alias inside the container.

## Calls made by the author (check these)

- **An invite grants writer or reader, never admin** (only admins invite: the owner decided that).
- **Readers can wrap keys for new devices** (`KEY_DISTRIBUTE`). It is key logistics for members an
  admin already let in, and it keeps a reader-only group from stalling a new device. Readers hold
  the key anyway.
- **All admins are equal**, and the creator is not special: an admin can demote or remove another
  admin.
- **The only admin cannot leave** while others remain (`LAST_ADMIN`). Account deletion instead
  promotes the earliest-joined writer, else reader.
- **Group limits from the best admin tier**, replacing the owner's tier, so the `owner_id` column
  goes away.
- **Members see each other's display names, not emails** (`UserRef`).
- **Plaintext `kind` and `object` columns on `group_log`**, so the server can enforce per-kind
  permissions. It already learns both from blob uploads and deletes.
- **GraphQL for everything but redirects, blob bytes, the Stripe webhook and the health check.**
- **Expected refusals are payload data** (`UserError` with an `ErrorCode` enum); auth and transport
  failures are top-level errors. Unions were rejected because a new union member breaks old
  clients.
- **No dataloader, no depth limiter, no persisted-query allowlist.** The caps bound N+1, the
  acyclic schema bounds depth, and the clients are ours but must not be tied to one server
  deploy.
- **genqlient's runtime import is rewritten** rather than forked. It is one generated line, checked
  by the regeneration diff.
- **The `me` query is the account export**, replacing the separate export endpoint.
- **Password reset is included.** It costs one token purpose and two mutations; without it a
  forgotten password locks the account forever. It can be cut if Google sign-in is enough.
- **Verifying asks for the password** (pre-hijack defence), and verifying logs you in.
- **Stateless access tokens** with a 15-minute tail after logout, plus refresh tokens in `sessions`.
- **Rate limits in Postgres** rather than Cloud Armor (~$6+/month) or process memory.
- **Three Go modules** (game, backend, share) joined by `replace`, never a committed `go.work`.
  `go.work` and `-modfile` cannot be combined, and the game's fast tests depend on `-modfile`.
- **`share` holds only what both binaries import**, which today is `cid`, `wire` and the `api`
  contract files. Client-only crypto stays in the game.
- **genqlient lives in its own tools module** (`scripts/apigen`), so the game's dependency list
  names no GraphQL module at all.
- **Existing game packages do not move**; the game module keeps its path and layout.
- **The catalog is one query result** with an etag until it nears 1 MiB.
- **Several publishers can list the same CID** (one row each, keyed `(cid, publisher_id)`, and a
  shared blob that goes with the last listing), because deterministic exports make identical bytes
  common.
- **An operator takedown script** (`takedown.sh`) is the only moderation tool in phase 1.
- **Clients keep log entries and enum values they do not recognise** instead of refusing them, so
  a newer client can never stall an older client's sync.

## Open (needs the owner)

- Tier numbers (placeholders above).
- The SMTP provider: any provider with SMTP submission and a free tier (Resend, Brevo, Amazon SES).
  It is only `SMTP_URL` plus DNS records, so it can be swapped by config.
