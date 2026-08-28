// Command golf runs the Mario Golf: Super Rush online servers (auth + secure) on the
// Nextendo NEX stack — a from-scratch NEX implementation with no third-party dependencies.
// It runs the auth and secure servers in one process.
//
// Two NEX servers run in one process:
//   - auth   (:8457)  TicketGranting — LoginEx issues the Kerberos ticket.
//   - secure (:61000) SecureConnection + matchmaking + NAT traversal + Utility. Golf is a
//     multiplayer sports title (up to 4 players per round, plus Golf Adventure's larger
//     Battle Golf lobbies), so real matchmaking is expected to matter here, unlike titles
//     whose online mode is a single DataStore-only feature.
//
// UNLIKE every other title in this fleet: Golf has no kinnay/NintendoClients wiki entry at
// all (checked directly, 2026-08-28 — neither "Golf" nor "Rush" appear anywhere on the Game
// Server List page), and static extraction from the game binary hasn't been possible yet
// either — the base game dump on hand was a malformed NSZ (real, reproducible 28-byte
// structural offset in every compressed section, confirmed against two independent
// implementations; not a corrupt/garbage file, but not decompressable with stock tooling
// either) and the only NSP available (the 2021 launch update alone) has no matching
// title.key locally, so LibHac can't decrypt its Program NCA either. So unlike ARMS/MK8/S2,
// **defaultAccessKey and defaultNexVersion below are placeholders, not measured values** —
// see the "NEX identity" section of README.md for exactly what's blocking each path and
// what unblocks them.
//
// Every value that isn't nailed down yet is behind an env var (GOLF_*) so it can be flipped
// without recompiling once real values are known. See example.env and README.md.
package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"hash/fnv"
	"os"
	"strconv"
	"strings"
	"time"

	nex "github.com/NextendoNetwork/nextendo-nex"
)

const (
	// defaultAccessKey: PLACEHOLDER, not a real measured value -- see the package comment
	// above and README.md's "NEX identity" section. A wrong access key fails SILENTLY at
	// SYN (the PRUDP-Lite packet signature simply won't match and the console retries
	// forever with no error on either side), so don't expect a login attempt to even reach
	// this server's logs until the real key is in here.
	defaultAccessKey = "00000000"

	// defaultNexVersion: PLACEHOLDER. Golf released June 2021, close in time to Mario Party
	// Superstars (Oct 2021, confirmed NEX 4.6.5) and well past the Pia 5.19 cutover, so
	// legacyPia() below defaults to false (the modern Switch shape) rather than guessing an
	// exact version number -- unlike the version const, getting legacyPia's boolean wrong
	// is the kind of guess this file's other titles have gotten away with safely.
	defaultNexVersion = 40605

	securePID     = 2
	sessionKeyLen = 32

	// golfTitleID: the base title, from the two NSP filenames on hand (title ID confirmed
	// from real file dumps, not a guess) -- the update is 0100C9C00E25C800.
	golfTitleID = "0100c9c00e25c000"
)

var (
	// accessKey / nexVersion — see the const block above for provenance.
	accessKey  = envOr("GOLF_ACCESS_KEY", defaultAccessKey)
	nexVersion = envOrInt("GOLF_NEX_VERSION", defaultNexVersion)

	// nextendoHost is the BARE IP the console will dial — NOT host:port. It is used only as
	// the "address" param of the secure station URL; the port comes from SECURE_PORT.
	nextendoHost = envOr("NEXTENDO_HOST", "127.0.0.1")
	// Ports chosen at deploy time: mario-strikers/mhgu had already claimed 8454-8456 and
	// 60014-60015 on the shared VPS despite not showing it in their own local example.env
	// (always check the box's actual listening sockets, not just sibling repos, before
	// picking a port). securePort is deliberately outside the box's 32768-60999 ephemeral
	// range (`cat /proc/sys/net/ipv4/ip_local_port_range`) -- every other title's secure
	// port sits inside that range and works fine in practice since they only bind once at
	// startup, but a fresh bind attempt can transiently collide with someone else's outbound
	// connection that happened to get assigned the same port a moment earlier (hit this
	// directly: sni-router's own ephemeral source port to nx-dauth was sitting on 60016
	// for the few seconds this server's first few restart attempts tried to claim it).
	authPort   = envOrInt("AUTH_PORT", 8457)
	securePort = envOrInt("SECURE_PORT", 61000)

	securePassword = envOr("NEXTENDO_SECURE_PASSWORD", "securepasswordplz1")
	certFile       = envOr("CERT_FILE", "cert.pem")
	keyFile        = envOr("KEY_FILE", "key.pem")

	// nextendoSecret signs "nx2." NEX login tokens issued by the account service. It MUST
	// be byte-identical to nextendo-account's secret or token validation fails.
	nextendoSecret = loadNextendoSecret()
	// requireAccount, when "1", rejects any login without a valid Nextendo token.
	requireAccount = os.Getenv("NEXTENDO_REQUIRE_ACCOUNT") == "1"
)

// --- Unverified wire-shape knobs, all placeholders pending a real client test -- see the
// --- package comment above. Each is an env var so a wrong guess costs a restart, not a
// --- recompile.

// stationScheme: "prudps" (PRUDP *Secure*) makes the console treat the secure server as
// authenticated and hand over its Kerberos ticket in CONNECT. With "prudp" the handshake
// completes but CONNECT carries an EMPTY payload — no ticket, no session key — and the title
// dies the moment Pia needs the session (menus still look fine). S2 + SSBU need "prudps";
// MK8 is the outlier that still uses "prudp".
func stationScheme() string { return envOr("GOLF_STATION_SCHEME", "prudps") }

// secureMinor: the PRUDP minor version the SECURE endpoint answers SYN with (the
// supported-functions option encodes minorVersion|supportedFunc<<8). The retail secure server
// answered 0 where nextendo-nex defaults to 5; with 5 the console sends CONNECT with plen=0
// and never hands over the ticket. NEVER force this on the auth endpoint — SSBU's auth
// stopped receiving logins entirely when that was done, which is why only the secure endpoint
// gets its own Settings object below.
func secureMinor() int { return envOrInt("GOLF_SECURE_MINOR", 0) }

// legacyPia: Golf's real Pia version hasn't been measured (see package comment). Defaults to
// the modern type=0x0B + Pa shape (SSBU/MK8) since Golf released June 2021, well past the
// Pia 5.19 cutover -- set GOLF_LEGACY_PIA=1 to try the legacy type=0x03 shape instead if
// SecureConnection.Register fails.
func legacyPia() bool { return envOr("GOLF_LEGACY_PIA", "0") != "0" }

func main() {
	settings := nex.NewSwitchSettings(accessKey, nexVersion)

	// --- Auth server (:8457) ---
	// The scheme of this station URL is how the client decides the target is a secure server
	// and therefore hands over its Kerberos ticket in CONNECT — see stationScheme() above.
	secureURL := nex.NewStationURL(stationScheme())
	secureURL.Set("address", nextendoHost)
	secureURL.SetInt("port", securePort)
	secureURL.SetInt("CID", 1)
	secureURL.SetInt("PID", securePID)
	secureURL.SetInt("sid", 1)
	secureURL.SetInt("stream", 10)
	secureURL.SetInt("type", 2) // public

	authEndpoint := nex.NewEndpoint(settings)
	authCfg := &nex.AuthConfig{
		Settings:         settings,
		SecurePID:        securePID,
		SecurePassword:   securePassword,
		SecureStationURL: secureURL,
		ServerName:       "Nextendo",
		SessionKeyLength: sessionKeyLen,
		ResolveUser:      resolveUser,
	}
	authEndpoint.Register(nex.ProtocolTicketGranting, authCfg.Handler())
	authEndpoint.OnRMC = logRMC("Auth")
	authServer := nex.NewServer(authEndpoint)

	// --- Secure server (:61000) ---
	// Its OWN settings, scoped on purpose: the auth (:8457) is a separate PRUDP server and
	// must keep the default minor version. Cf. secureMinor().
	secureSettings := nex.NewSwitchSettings(accessKey, nexVersion)
	secureSettings.PrudpMinorVersion = secureMinor()
	secureEndpoint := nex.NewEndpoint(secureSettings)
	secureEndpoint.SetSecureAccount(securePassword, securePID)

	mm := nex.NewMatchmaking()

	scCfg := nex.LegacyPiaConfig()
	if !legacyPia() {
		scCfg = nex.SwitchPia519Config()
	}
	secureEndpoint.Register(nex.ProtocolSecureConnection, nex.SecureConnectionHandlerWithConfig(scCfg))
	secureEndpoint.Register(nex.ProtocolMatchmakeExtension, mm.ExtensionHandler())
	secureEndpoint.Register(nex.ProtocolMatchMaking, mm.MatchMakingHandler())
	secureEndpoint.Register(nex.ProtocolMatchMakingExt, mm.MatchMakingExtHandler())
	secureEndpoint.Register(nex.ProtocolNATTraversal, nex.NATTraversalHandler())
	secureEndpoint.Register(nex.ProtocolUtility, nex.UtilityHandler())
	// Ranking (0x70): golf scoring is an obvious candidate for real per-round leaderboards,
	// but whether Golf actually calls Ranking or just DataStore for that hasn't been
	// observed yet -- registered with the generic handler for now, same starting point
	// arms/MPS began from before any stub was written.
	secureEndpoint.Register(nex.ProtocolRanking, nex.RankingHandler())
	// DataStore (0x73): not yet stubbed. Register once a real client's calls show what it
	// needs -- see mario-party-superstars's datastore_stub.go for the pattern to follow.

	logSecure := logRMC("Secure")
	secureEndpoint.OnRMC = func(c *nex.Connection, req *nex.RMCMessage) {
		logSecure(c, req)
		noteRMC(c, req)         // feed the monitoring dashboard
		notePresenceSeen(c.PID) // any packet from a PID = that account is playing Golf now
	}
	secureEndpoint.OnConnect = func(c *nex.Connection) {
		fmt.Printf("[Golf Secure] connected pid=%d id=%d addr=%s\n", c.PID, c.ID, c.RemoteAddr)
	}
	// Drop the player's lobbies when the connection dies. A gathering is otherwise only
	// removed when the client politely calls UnregisterGathering / EndParticipation, so a
	// client that crashes or errors out leaks its lobby forever.
	secureEndpoint.OnDisconnect = func(c *nex.Connection) {
		mm.RemovePlayer(c.PID)
	}
	secureServer := nex.NewServer(secureEndpoint)

	// Éviction automatique des connexions mortes + monitoring /api/stats.
	secureEndpoint.StartReaper()
	go startDashboard(secureEndpoint, mm)
	startPresenceReporter()

	// When the auth is fronted by a TLS-passthrough proxy (Traefik on the shared :443),
	// enable PROXY protocol so the auth sees the console's REAL IP. Not used locally.
	proxyProto := os.Getenv("NEXTENDO_PROXY_PROTOCOL") == "1"
	go func() {
		fmt.Printf("[Golf Auth] listening WSS :%d (proxyProto=%v, secure URL -> %s)\n", authPort, proxyProto, secureURL.String())
		var err error
		if proxyProto {
			err = authServer.ListenSecureProxy(authPort, certFile, keyFile)
		} else {
			err = authServer.ListenSecure(authPort, certFile, keyFile)
		}
		if err != nil {
			fmt.Printf("[Golf Auth] stopped: %v\n", err)
		}
	}()

	fmt.Printf("[Golf Secure] listening WSS :%d (accessKey=%s nexVersion=%d scheme=%s minor=%d legacyPia=%v title=%s)\n",
		securePort, accessKey, nexVersion, stationScheme(), secureMinor(), legacyPia(), golfTitleID)
	if err := secureServer.ListenSecure(securePort, certFile, keyFile); err != nil {
		fmt.Printf("[Golf Secure] stopped: %v\n", err)
	}
}

// resolveUser maps a LoginEx username to an account. A valid "nx2." Nextendo token
// resolves to its persistent PID; anything else gets a stable anonymous PID derived from
// the username (so the same console keeps the same identity).
//
// For a LOCAL test: Citron sends the bare decimal PID from config/nextendo_account.txt
// (e.g. 1800003542). That is >= 1800000000 and < 1810000000, so branch 2 takes it verbatim,
// resolveNSAtoPID (the only FAIL-CLOSED path) is never reached, and the single account call
// nextendoOnlineCheck fails OPEN on any transport error — no account server required.
func resolveUser(username string, _ []byte) (uint64, []byte, bool) {
	// The source key encrypts the client ticket and is handed back as pSourceKey, so the
	// console decrypts it. It MUST be 32 bytes (the Switch kerberos key size).
	sk := sha256.Sum256([]byte("nextendo-src:" + username))
	sourceKey := sk[:]

	// 1. Signed nx2 token → the account's PERSISTENT PID (+ online gates).
	if pid, ok := nextendoPIDFromToken(username); ok {
		if allow, reason := nextendoOnlineCheck(pid, "ryujinx"); !allow {
			fmt.Printf("[Auth] pid=%d online REFUSÉ (%s)\n", pid, reason)
			return 0, nil, false
		}
		return pid, sourceKey, true
	}

	// 2. Numeric username. The emulator's "Connexion Nextendo" button sends the account's
	// OWN PID; a real CFW Switch sends its console baasUserID (a large NSA id) instead,
	// which we resolve to the account PID. Using the account PID verbatim keeps the NEX
	// identity = the account the game knows itself by (hashing it breaks Pia's
	// self-recognition → 2618-562 SessionKeepFailed).
	if n, err := strconv.ParseUint(username, 10, 64); err == nil && n >= 1800000000 {
		// FAILLE D AUTHENTIFICATION CONNUE. Ce chemin accepte un PID NU comme identite :
		// aucun jeton, aucune signature. Les PID etant sequentiels depuis 1800000001, il
		// suffit d envoyer le numero d un autre membre pour jouer sous son identite — et,
		// via la garde « un seul endroit », l empecher lui-meme de jouer.
		// On ne peut pas l interdire sechement : l emulateur distribue envoie precisement
		// ce PID nu. Le refus est donc derriere un interrupteur, a activer quand une build
		// envoyant le jeton nx2 signe sera deployee. En attendant on journalise chaque usage.
		if requireSignedToken() {
			fmt.Printf("[Auth] pid=%d REFUSE : identite par PID nu desactivee (jeton nx2 signe requis)\n", n)
			return 0, nil, false
		}
		fmt.Printf("[Auth] pid=%d identite par PID NU (non authentifiee — cf. NEXTENDO_REQUIRE_SIGNED_TOKEN)\n", n)
		pid, kind := n, "ryujinx"
		if n >= 1810000000 { // vraie Switch : NSA id -> PID de compte (online = comptes Nextendo UNIQUEMENT)
			kind = "switch"
			rp, st := resolveNSAtoPID(n)
			switch st {
			case nsaOK:
				pid = rp
				fmt.Printf("[Auth] NSA %d -> account pid=%d\n", n, pid)
			case nsaUnknown:
				fmt.Printf("[Auth] NSA %d REFUSÉ (aucun compte Nextendo)\n", n)
				return 0, nil, false
			case nsaUnreachable:
				fmt.Printf("[Auth] NSA %d REFUSÉ (serveur compte injoignable)\n", n)
				return 0, nil, false
			}
		}
		// GATES online : #6 e-mail vérifié + #5 un seul endroit + compte inconnu/désactivé.
		if allow, reason := nextendoOnlineCheck(pid, kind); !allow {
			fmt.Printf("[Auth] pid=%d online REFUSÉ (%s)\n", pid, reason)
			return 0, nil, false
		}
		return pid, sourceKey, true
	}

	// 3. Anonymous / no Nextendo identity. When requireAccount is on, online REQUIRES a
	// Nextendo account → reject (the game can't enter online mode).
	if requireAccount {
		fmt.Printf("[Auth] login anonyme REFUSÉ (compte Nextendo requis): %q\n", username)
		return 0, nil, false
	}
	return anonymousPID(username), sourceKey, true
}

// nextendoPIDFromToken validates a "nx2.<b64(pid.username.expiry)>.<b64(hmac)>" token
// signed by the account service (HMAC-SHA256, "nex:" prefix).
func nextendoPIDFromToken(s string) (uint64, bool) {
	if len(nextendoSecret) == 0 || !strings.HasPrefix(s, "nx2.") {
		return 0, false
	}
	parts := strings.Split(s[len("nx2."):], ".")
	if len(parts) != 2 {
		return 0, false
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return 0, false
	}
	mac := hmac.New(sha256.New, nextendoSecret)
	mac.Write([]byte("nex:" + string(raw)))
	want := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(want), []byte(parts[1])) {
		return 0, false
	}
	f := strings.SplitN(string(raw), ".", 3) // pid.username.expiry
	if len(f) != 3 {
		return 0, false
	}
	pid, err := strconv.ParseUint(f[0], 10, 64)
	if err != nil {
		return 0, false
	}
	if exp, err := strconv.ParseInt(f[2], 10, 64); err != nil || time.Now().Unix() > exp {
		return 0, false
	}
	return pid, true
}

// loadNextendoSecret loads the shared NEX-token signing secret the SAME way
// nextendo-account does (its loadSecret): env NEXTENDO_SECRET as raw bytes if set,
// otherwise hex-decode the shared key file. The deployed account has no env → it
// hex-decodes the file, so we must too or the HMAC won't match.
func loadNextendoSecret() []byte {
	if v := os.Getenv("NEXTENDO_SECRET"); v != "" {
		return []byte(v)
	}
	path := envOr("NEXTENDO_SECRET_FILE", "nextendo_secret.key")
	if b, err := os.ReadFile(path); err == nil {
		if dec, derr := hex.DecodeString(strings.TrimSpace(string(b))); derr == nil && len(dec) >= 16 {
			return dec
		}
	}
	return nil
}

// anonymousPID derives a stable PID in the NEX user range from a username.
func anonymousPID(username string) uint64 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(username))
	return 1800000000 + uint64(h.Sum32()%100000000)
}

func logRMC(tag string) func(*nex.Connection, *nex.RMCMessage) {
	return func(c *nex.Connection, req *nex.RMCMessage) {
		fmt.Printf("[Golf %s] pid=%d proto=%#x method=%d call=%d\n", tag, c.PID, req.Protocol, req.Method, req.CallID)
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envOrInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

// requireSignedToken : quand true, seule une identite prouvee par un jeton nx2 SIGNE est
// acceptee au LoginEx ; un PID nu est refuse. Desactive par defaut car l emulateur
// actuellement distribue envoie encore le PID nu — a activer apres la prochaine release.
func requireSignedToken() bool {
	v := os.Getenv("NEXTENDO_REQUIRE_SIGNED_TOKEN")
	return v == "1" || v == "true"
}
