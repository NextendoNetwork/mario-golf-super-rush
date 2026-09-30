package main

import (
	"bytes"
	nex "github.com/NextendoNetwork/nextendo-nex"
	"testing"
)

func TestGolfRankedOpenWithoutChangingOtherData(t *testing.T) {
	s := nex.NewSwitchSettings(defaultAccessKey, defaultNexVersion)
	for _, tc := range []struct {
		mode     uint32
		password string
		want     bool
	}{{2, "", true}, {1, "", false}, {2, "secret", false}} {
		p := &nex.AutoMatchmakeParam{Session: nex.MatchmakeSession{Gathering: nex.Gathering{MaxParticipants: 4}, GameMode: tc.mode, Attribs: []uint32{3, 0, 320, 7, 0, 1073769413}, UserPassword: tc.password, UserPasswordEnabled: tc.password != ""}}
		o := nex.NewStreamOut(s)
		o.Add(p)
		body := append(o.Bytes(), []byte{10, 20, 30, 40}...)
		original := append([]byte(nil), body...)
		c := nex.NewConnection(nex.NewEndpoint(s), "127.0.0.1:1", func([]byte) {})
		h := golfMatchmakingHandler(func(_ *nex.Connection, r *nex.RMCMessage) *nex.RMCMessage {
			var got nex.AutoMatchmakeParam
			in := nex.NewStreamIn(r.Body, s)
			in.Extract(&got)
			if in.Err() != nil || got.Session.OpenParticipation != tc.want {
				t.Fatalf("mode %d: open=%v err=%v", tc.mode, got.Session.OpenParticipation, in.Err())
			}
			changes := 0
			for i, b := range original {
				if b != r.Body[i] {
					changes++
				}
			}
			want := 0
			if tc.want {
				want = 1
			}
			if changes != want {
				t.Fatalf("changed %d bytes, want %d", changes, want)
			}
			return nil
		})
		h(c, nex.NewRMCRequest(s, nex.ProtocolMatchmakeExtension, nex.MethodAutoMatchmakeWithParamPostpone, 1, body))
		if !bytes.Equal(body, original) {
			t.Fatal("mutated original request")
		}
	}
}
func TestGolfSimultaneousRankedSearchesShareLobby(t *testing.T) {
 s:=nex.NewSwitchSettings(defaultAccessKey,defaultNexVersion)
 ep:=nex.NewEndpoint(s); mm:=nex.NewMatchmaking(); h:=golfMatchmakingHandler(mm.ExtensionHandler())
 var gid uint32
 for j:=0;j<2;j++ {
  c:=nex.NewConnection(ep,"127.0.0.1:1",func([]byte){});c.PID=uint64(100+j)
  p:=&nex.AutoMatchmakeParam{Session:nex.MatchmakeSession{Gathering:nex.Gathering{MaxParticipants:4},GameMode:2,Attribs:[]uint32{3,0,320,7,0,1073769413}}}
  o:=nex.NewStreamOut(s);o.Add(p)
  r:=h(c,nex.NewRMCRequest(s,nex.ProtocolMatchmakeExtension,nex.MethodAutoMatchmakeWithParamPostpone,1,o.Bytes()))
  if r==nil || r.IsError {t.Fatal("matchmaking failed")}
  var room nex.MatchmakeSession;in:=nex.NewStreamIn(r.Body,s);in.Extract(&room)
  if in.Err()!=nil {t.Fatal(in.Err())}
  if j==0 {gid=room.ID} else if room.ID!=gid || room.NumParticipants!=2 {t.Fatalf("separate rooms: %v",room)}
 }
}
