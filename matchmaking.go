package main

import (
	"encoding/binary"
	nex "github.com/NextendoNetwork/nextendo-nex"
)

func golfMatchmakingHandler(next nex.RMCHandler) nex.RMCHandler {
	return func(c *nex.Connection, req *nex.RMCMessage) *nex.RMCMessage {
		if req.Method == nex.MethodAutoMatchmakeWithParamPostpone {
			var p nex.AutoMatchmakeParam
			in := nex.NewStreamIn(req.Body, c.Settings)
			in.Extract(&p)
			if in.Err() == nil && p.Session.GameMode == 2 && !p.Session.OpenParticipation && !p.Session.UserPasswordEnabled && p.Session.UserPassword == "" && p.Session.Codeword == "" {
				// Framed AutoMatchmakeParam contains a Gathering level followed
				// by MatchmakeSession. Its open byte follows mode and attributes.
				at := -1
				if c.Settings.StructHeader && len(req.Body) >= 10 {
					gatheringSize := uint64(binary.LittleEndian.Uint32(req.Body[6:10]))
					offset := uint64(5+5+5+4+4) + gatheringSize + uint64(len(p.Session.Attribs))*4
					if offset < uint64(len(req.Body)) {
						at = int(offset)
					}
				}
				if at >= 0 && req.Body[at] == 0 {
					copyReq := *req
					copyReq.Body = append([]byte(nil), req.Body...)
					copyReq.Body[at] = 1
					req = &copyReq
				}
			}
		}
		return next(c, req)
	}
}
