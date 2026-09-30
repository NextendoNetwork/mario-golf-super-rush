# Golf vendored networking change

Build with `go build -mod=vendor .`. The vendored nextendo-nex dependency contains a Golf-specific opt-in in natbridge.go, enabled by main.go through GOLF_PRESERVE_REPORTED_UDP.

Completed ReplaceURL records with CID, a nonzero UDP port and NAT flags retain their own public and local ports. The existing station formatting and ordering still apply. This prevents the IP-only NNCS cache from substituting another player's port when two clients share a public address.

Do not regenerate vendor without carrying this patch and natbridge_golf_test.go forward. Validate with `go test github.com/NextendoNetwork/nextendo-nex -run TestGolfSharedNAT` and `go test ./...`.

main.go also enables PreservePiaStationIdentity, which bypasses this bridge for session URLs; the patch stays for any path that still uses it. Two players joining the same Ranked lobby was confirmed live on 2026-09-30 with that setting.
