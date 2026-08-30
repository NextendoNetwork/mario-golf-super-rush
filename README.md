<h1 align="center">mario-golf-super-rush</h1>

<p align="center">
  <b>Nextendo Network game server for Mario Golf: Super Rush.</b>
</p>

<p align="center">
  <img src="https://img.shields.io/badge/license-PolyForm%20Shield%201.0.0-orange" alt="License: PolyForm Shield 1.0.0">
  <img src="https://img.shields.io/badge/go-1.23%2B-00ADD8" alt="Go 1.23+">
</p>

---

## What is this?

The NEX game server for **Mario Golf: Super Rush** (title ID `0100C9C00E25C000`) on
[Nextendo Network](https://nextendo.network). It handles authentication and matchmaking,
speaking the same NEX protocol the retail servers did.

It is built on the [**nextendo-nex**](https://github.com/NextendoNetwork/nextendo-nex) core
(PRUDP transport, RMC layer, common service protocols) and follows the same shape as
[`arms`](https://github.com/NextendoNetwork/arms): auth + secure NEX endpoints in one process,
P2P gameplay once matched. Golf is a real multiplayer sports title (up to 4 players per round,
plus Golf Adventure's larger Battle Golf lobbies), so matchmaking is expected to matter here
more than in titles whose online mode is a single DataStore feature.

**Status: playable.** `GOLF_ACCESS_KEY` is a confirmed real value — see below.

## NEX identity

### 1. Access key — CONFIRMED, not a guess

Every other title in this fleet got its access key from either the
[kinnay/NintendoClients wiki Game Server List](https://github.com/kinnay/NintendoClients/wiki/Game-Server-List)
or by extracting it directly from the game binary. Both were blocked for Golf:

So the access key was found the same way MPS's was, not by reading the binary: the DNS-resolve
hostname the console asks to resolve when attempting online play
(`g211a3f00-lp1.s.n.srv.nintendo.net`) was captured, `sni-router` was pointed at this server for
that hostname, and a real connection attempt was let through. It failed (the placeholder key
doesn't match), but the failure itself hands over exactly what's needed: the PRUDP-Lite packet
signature is a pure function of `(accessKey, connectionSig)` —
`HMAC-MD5(MD5(accessKey), MD5(accessKey)+connectionSig)` — and since access keys are always
exactly 8 hex digits (a 2^32 search space), brute-forcing all candidates against that one real
captured `(connectionSig, client signature)` pair found the exact key with zero ambiguity:
**`1cb8027c`**. (Note `g211a3f00`, the hostname value, is a separate Game Server ID — not the
access key itself, same distinction MPS's README documents.)

### 2. NEX version and Pia wire shape — still guesses

`GOLF_NEX_VERSION` defaults to `40605` (NEX 4.6.5) on the reasoning that Golf (June 2021)
released close in time to Mario Party Superstars (Oct 2021, confirmed 4.6.5) — an era guess,
not a measurement. `GOLF_LEGACY_PIA` defaults to `0` (the modern Pia 5.19+ shape) since June
2021 is well past that cutover. Both are overridable without a recompile; try flipping
`GOLF_LEGACY_PIA` first if `SecureConnection.Register` fails now that the real access key is
in place.

### 3. DataStore / Ranking

Neither is stubbed yet. `Ranking` (`0x70`) is registered with the generic
`nex.RankingHandler()`; whether Golf actually needs a title-specific `DataStore` (`0x73`)
stub for post-round score submission the way ARMS and MPS do is unknown until a real client's
calls show what it expects — see `mario-party-superstars`'s `datastore_stub.go` for the
pattern to follow once that's known.

## Running

```sh
cp example.env .env    # then edit .env
go run .
```

Configuration is entirely through environment variables — see [`example.env`](example.env). No
secrets are baked into the source.

## What this is not

This server ships **no** Nintendo code, keys, or copyrighted assets. It is an independent
reimplementation for use with a community-run replacement service, not affiliated with, endorsed by,
or associated with Nintendo. The NEX access key it uses (once found) is a well-known per-title value
derivable from the game itself, not a secret.

## License

Released under the **[PolyForm Shield License 1.0.0](LICENSE.md)** — source-available: read, use,
modify, and self-host, but do not use it to provide a product that competes with Nextendo Network.
