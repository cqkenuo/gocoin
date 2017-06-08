package network

import (
	"fmt"
	"time"
	"sort"
	"sync/atomic"
	"crypto/rand"
	"github.com/piotrnar/gocoin/client/common"
)

const (
	PingHistoryLength = 20
	PingAssumedIfUnsupported = 4999 // ms
)


func (c *OneConnection) HandlePong() {
	ms := time.Now().Sub(c.LastPingSent) / time.Millisecond
	if common.DebugLevel>1 {
		println(c.PeerAddr.Ip(), "pong after", ms, "ms", time.Now().Sub(c.LastPingSent).String())
	}
	if ms==0 {
		//println(c.ConnID, "Ping returned after 0ms")
		ms = 1
	}
	c.Mutex.Lock()
	c.X.PingHistory[c.X.PingHistoryIdx] = int(ms)
	c.X.PingHistoryIdx = (c.X.PingHistoryIdx+1)%PingHistoryLength
	c.PingInProgress = nil
	c.Mutex.Unlock()
}


// Returns (median) average ping
// Make sure to called it within c.Mutex.Lock()
func (c *OneConnection) GetAveragePing() int {
	if c.Node.Version>60000 {
		var pgs[PingHistoryLength] int
		var act_len int
		for _, p := range c.X.PingHistory {
			if p!=0 {
				pgs[act_len] = p
				act_len++
			}
		}
		if act_len==0 {
			return 0
		}
		sort.Ints(pgs[:act_len])
		return pgs[act_len/2]
	} else {
		return PingAssumedIfUnsupported
	}
}


type conn_list_to_drop []struct {
	c *OneConnection
	ping int
	blks int
	txs int
	mins int
}

func (l conn_list_to_drop) Len() int {
	return len(l)
}

func (l conn_list_to_drop) Less(a, b int) bool {
	// If any of the two is connected for less than one hour, just compare the ping
	if l[a].mins<60 || l[b].mins<60 {
		return l[a].ping > l[b].ping
	}

	if l[a].blks == l[b].blks {
		if l[a].txs == l[b].txs {
			return l[a].ping > l[b].ping
		}
		return l[a].txs < l[b].txs
	}
	return l[a].blks < l[b].blks
}

func (l conn_list_to_drop) Swap(a, b int) {
	l[a], l[b] = l[b], l[a]
}


// This function should be called only when OutConsActive >= MaxOutCons
func drop_worst_peer() bool {
	var list conn_list_to_drop
	var cnt int
	var any_ping bool

	Mutex_net.Lock()
	defer Mutex_net.Unlock()

	now := time.Now()
	list = make(conn_list_to_drop, len(OpenCons))
	for _, v := range OpenCons {
		v.Mutex.Lock()
		// do not drop peers that connected just recently
		if time_online := now.Sub(v.X.ConnectedAt); time_online >= common.DropSlowestEvery {
			list[cnt].c = v
			list[cnt].ping = v.GetAveragePing()
			list[cnt].blks = len(v.blocksreceived)
			list[cnt].txs = v.X.TxsReceived
			list[cnt].mins = int(time_online/time.Minute)
			if list[cnt].ping>0 {
				any_ping = true
			}
			cnt++
		}
		v.Mutex.Unlock()
	}
	if !any_ping || cnt==0 {
		return false
	}
	sort.Sort(list)
	for _, v := range list {
		if v.c.X.Incomming {
			if InConsActive+2 > atomic.LoadUint32(&common.CFG.Net.MaxInCons) {
				common.CountSafe("PeerInDropped")
				fmt.Printf("Drop incomming id:%d  blks:%d  txs:%d  ping:%d  mins:%d\n> ",
					v.c.ConnID, v.blks, v.txs, v.ping, v.mins)
				v.c.Disconnect()
				return true
			}
		} else {
			if OutConsActive+2 > atomic.LoadUint32(&common.CFG.Net.MaxOutCons) {
				common.CountSafe("PeerOutDropped")
				fmt.Printf("Drop outgoing id:%d  blks:%d  txs:%d  ping:%d  mins:%d\n> ",
					v.c.ConnID, v.blks, v.txs, v.ping, v.mins)
				v.c.Disconnect()
				return true
			}
		}
	}
	return false
}


func (c *OneConnection) TryPing() bool {
	if common.PingPeerEvery==0 {
		return false // pinging disabled in global config
	}

	if c.Node.Version<=60000 {
		return false // insufficient protocol version
	}

	if time.Now().Before(c.LastPingSent.Add(common.PingPeerEvery)) {
		return false // not yet...
	}

	if c.PingInProgress!=nil {
		if common.DebugLevel > 0 {
			println(c.PeerAddr.Ip(), "ping timeout")
		}
		common.CountSafe("PingTimeout")
		c.HandlePong()  // this will set PingInProgress to nil
	}

	c.PingInProgress = make([]byte, 8)
	rand.Read(c.PingInProgress[:])
	c.SendRawMsg("ping", c.PingInProgress)
	c.LastPingSent = time.Now()
	//println(c.PeerAddr.Ip(), "ping...")
	return true
}
