package main

import nex "github.com/NextendoNetwork/nextendo-nex"

// Standard NEX Ranking (0x70) methods Golf calls that nextendo-nex main's handler, written
// for Mario Kart 8's methods, doesn't answer. Unanswered, Ranked Match fails with 2306-0103.
// Same responses as the nextendo-nex fork already running Mario Tennis Aces.
const (
	methodUploadScore          uint32 = 0x1
	methodGetRanking           uint32 = 0x9
	methodGetCachedTopXRanking uint32 = 0xE
)

func golfRankingHandler() nex.RMCHandler {
	base := nex.RankingHandler()
	return func(conn *nex.Connection, req *nex.RMCMessage) *nex.RMCMessage {
		s := conn.Settings
		switch req.Method {
		case methodUploadScore:
			return nex.NewRMCSuccess(s, nex.ProtocolRanking, req.Method, req.CallID, nil)
		case methodGetRanking:
			out := nex.NewStreamOut(s)
			out.Add(&rankingResult{SinceTime: nex.NowDateTime().Value()})
			return nex.NewRMCSuccess(s, nex.ProtocolRanking, req.Method, req.CallID, out.Bytes())
		case methodGetCachedTopXRanking:
			now := nex.NowDateTime().Value()
			out := nex.NewStreamOut(s)
			out.Add(&rankingCachedResult{SinceTime: now, CreatedTime: now, ExpiredTime: now})
			return nex.NewRMCSuccess(s, nex.ProtocolRanking, req.Method, req.CallID, out.Bytes())
		}
		return base(conn, req)
	}
}

// rankingResult is RankingResult{data List<RankingRankData>, total u32, since_time DateTime},
// sent with no entries yet. It is a Structure, so it needs its own struct header on the wire.
type rankingResult struct {
	Total     uint32
	SinceTime uint64
}

func (r *rankingResult) Levels() []nex.Level {
	return []nex.Level{{Save: func(o *nex.StreamOut) {
		nex.WriteList(o, []struct{}{}, func(*nex.StreamOut, struct{}) {})
		o.U32(r.Total)
		o.DateTime(r.SinceTime)
	}}}
}

// rankingCachedResult inherits RankingResult, so it is two struct-header levels.
type rankingCachedResult struct {
	Total       uint32
	SinceTime   uint64
	CreatedTime uint64
	ExpiredTime uint64
	MaxLength   uint8
}

func (r *rankingCachedResult) Levels() []nex.Level {
	return []nex.Level{
		{Save: func(o *nex.StreamOut) {
			nex.WriteList(o, []struct{}{}, func(*nex.StreamOut, struct{}) {})
			o.U32(r.Total)
			o.DateTime(r.SinceTime)
		}},
		{Save: func(o *nex.StreamOut) {
			o.DateTime(r.CreatedTime)
			o.DateTime(r.ExpiredTime)
			o.U8(r.MaxLength)
		}},
	}
}
