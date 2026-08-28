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

**Status: scaffolded, not yet playable.** The server builds and runs, but `GOLF_ACCESS_KEY` is
a placeholder, not a real value — see below.

## NEX identity — currently blocked, not yet a guess

Every other title in this fleet got its access key from either the
[kinnay/NintendoClients wiki Game Server List](https://github.com/kinnay/NintendoClients/wiki/Game-Server-List)
or by extracting it directly from the game binary. Both are blocked for Golf right now:

1. **No wiki entry.** Checked directly (2026-08-28): neither "Golf" nor "Rush" appear anywhere
   on the Game Server List page. Worth re-checking periodically in case that changes.
2. **No working game dump to extract from, yet.** The base game copy on hand is an `.nsz` with
   a real, reproducible structural defect — every compressed section's true content starts
   28 bytes later than the standard NCZ format expects (confirmed independently against both
   the reference Python `nsz` tool and LibHac.NSZ, the C# library other tools like NxFileViewer
   use — not a coincidence or a tool-version quirk), *and* the tail of its largest content
   file has ~5 blocks with a declared compressed size of 0 (real, small-scale data loss, not
   a parsing artifact). A clean redump is the fix in progress. The only NSP on hand otherwise
   (the 2021 launch update, standalone) has no matching title key available locally, so even
   LibHac can't decrypt its Program NCA to search it statically.

Once either a working base-game dump or a matching title key is available, extraction should
follow the pattern already proven on this fleet: static ADRP/string-reference scanning first
(how `arms`, SMB35, and SMO's keys were found), and if that comes up empty the way it did for
MPS, a live capture is the fallback — either the DNS-resolve hostname citron/Ryujinx asks for
when the game attempts to connect (`g<accesskey>-lp1...` for most titles, though MPS's case
showed that hostname can also just be the Game Server ID, not the access key — confirm which
before trusting it), or a captured real PRUDP CONNECT signature brute-forced the way MPS's key
ultimately was (see that repo's README for the exact method).

## Wire-shape defaults

`GOLF_NEX_VERSION` defaults to `40605` (NEX 4.6.5) on the reasoning that Golf (June 2021)
released close in time to Mario Party Superstars (Oct 2021, confirmed 4.6.5) — an era guess,
not a measurement. `GOLF_LEGACY_PIA` defaults to `0` (the modern Pia 5.19+ shape) since June
2021 is well past that cutover. Both are overridable without a recompile; try flipping
`GOLF_LEGACY_PIA` first if `SecureConnection.Register` fails once a real access key is in
place.

## DataStore / Ranking

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
