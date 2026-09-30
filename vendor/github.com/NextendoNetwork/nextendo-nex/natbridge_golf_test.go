package nex

import (
	"testing"
	"time"
)

func TestGolfSharedNATPreservesEachEndpoint(t *testing.T) {
	t.Setenv("GOLF_PRESERVE_REPORTED_UDP", "1")
	natCacheMu.Lock()
	old, oldRead := natCache, natCacheRead
	natCache = map[string]int{"203.0.113.7": 60665}
	natCacheRead = time.Now()
	natCacheMu.Unlock()
	defer func() { natCacheMu.Lock(); natCache = old; natCacheRead = oldRead; natCacheMu.Unlock() }()
	for _, port := range []int{63988, 60665} {
		local := ParseStationURL("prudps:/address=10.0.0.123;port=50000;CID=3721808193;RVCID=2;natf=17;natm=1")
		public := ParseStationURL("prudps:/address=203.0.113.7;port=1;CID=3721808193;RVCID=2;natf=17;natm=1;type=11")
		public.SetInt("port", port)
		got, status := natBridgeStations([]*StationURL{local, public}, true)
		if status != bridgeOK || len(got) != 2 {
			t.Fatalf("status=%v urls=%v", status, got)
		}
		if got[0].GetInt("port") != port || got[0].Get("address") != "203.0.113.7" || got[1].GetInt("port") != 50000 {
			t.Fatalf("mixed endpoints: %v", got)
		}
		if local.GetInt("port") != 50000 || public.GetInt("port") != port {
			t.Fatal("mutated input")
		}
	}
}
